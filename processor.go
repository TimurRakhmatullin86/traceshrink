package traceshrink

import (
	"context"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type decision int

const (
	decisionPending decision = iota
	decisionKeep
	decisionDrop
)

type traceData struct {
	spans      ptrace.Traces
	arrivalAt  time.Time
	decision   decision
	spanCount  int
	totalCost  float64
	hasError   bool
	maxDurNano int64
}

type traceShrinkProcessor struct {
	logger *zap.Logger
	cfg    *Config
	next   consumer.Traces

	mu     sync.Mutex
	traces map[pcommon.TraceID]*traceData

	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func newTraceShrinkProcessor(logger *zap.Logger, cfg *Config, next consumer.Traces) *traceShrinkProcessor {
	return &traceShrinkProcessor{
		logger: logger,
		cfg:    cfg,
		next:   next,
		traces: make(map[pcommon.TraceID]*traceData),
		stopCh: make(chan struct{}),
	}
}

func (p *traceShrinkProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func (p *traceShrinkProcessor) Start(_ context.Context, _ component.Host) error {
	p.wg.Add(1)
	go p.sweepLoop()
	return nil
}

func (p *traceShrinkProcessor) Shutdown(_ context.Context) error {
	p.stopOnce.Do(func() { close(p.stopCh) })
	p.wg.Wait()
	return nil
}

func (p *traceShrinkProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		ilss := rs.ScopeSpans()
		for j := 0; j < ilss.Len(); j++ {
			spans := ilss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				traceID := span.TraceID()

				td, ok := p.traces[traceID]
				if !ok {
					if uint64(len(p.traces)) >= p.cfg.NumTraces {
						p.evictOldest()
					}
					td = &traceData{
						spans:     ptrace.NewTraces(),
						arrivalAt: time.Now(),
						decision:  decisionPending,
					}
					p.traces[traceID] = td
				}

				td.spanCount++

				cost := p.extractCost(span)
				td.totalCost += cost

				if span.Status().Code() == ptrace.StatusCodeError {
					td.hasError = true
				}

				durNano := int64(span.EndTimestamp() - span.StartTimestamp())
				if durNano > td.maxDurNano {
					td.maxDurNano = durNano
				}

				if td.decision == decisionPending {
					td.decision = p.evaluate(td, span)
				}

				p.appendSpan(td, rs, ilss.At(j), span)
			}
		}
	}
	return nil
}

func (p *traceShrinkProcessor) evaluate(td *traceData, span ptrace.Span) decision {
	if p.cfg.KeepErrors && td.hasError {
		return decisionKeep
	}
	if p.cfg.CostThreshold > 0 && td.totalCost >= p.cfg.CostThreshold {
		return decisionKeep
	}
	if p.cfg.DurationThreshold > 0 && td.maxDurNano >= p.cfg.DurationThreshold.Nanoseconds() {
		return decisionKeep
	}
	if len(p.cfg.KeepAttributes) > 0 && p.hasAnyAttribute(span) {
		return decisionKeep
	}
	return decisionPending
}

func (p *traceShrinkProcessor) hasAnyAttribute(span ptrace.Span) bool {
	attrs := span.Attributes()
	for _, attr := range p.cfg.KeepAttributes {
		if _, ok := attrs.Get(attr); ok {
			return true
		}
	}
	return false
}

func (p *traceShrinkProcessor) extractCost(span ptrace.Span) float64 {
	attr := p.cfg.CostAttribute
	if attr == "" {
		attr = "llm.cost"
	}
	v, ok := span.Attributes().Get(attr)
	if !ok {
		return 0
	}
	switch v.Type() {
	case pcommon.ValueTypeDouble:
		return v.Double()
	case pcommon.ValueTypeInt:
		return float64(v.Int())
	case pcommon.ValueTypeStr:
		f, err := strconv.ParseFloat(v.Str(), 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

func (p *traceShrinkProcessor) appendSpan(td *traceData, srcRS ptrace.ResourceSpans, srcILS ptrace.ScopeSpans, span ptrace.Span) {
	destRS := td.spans.ResourceSpans().AppendEmpty()
	srcRS.Resource().CopyTo(destRS.Resource())
	destILS := destRS.ScopeSpans().AppendEmpty()
	srcILS.Scope().CopyTo(destILS.Scope())
	destSpan := destILS.Spans().AppendEmpty()
	span.CopyTo(destSpan)
}

func (p *traceShrinkProcessor) sweepLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.cfg.DecisionWait / 2)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			p.flushAll()
			return
		case <-ticker.C:
			p.sweep()
		}
	}
}

func (p *traceShrinkProcessor) sweep() {
	p.mu.Lock()
	now := time.Now()
	var toKeep []ptrace.Traces
	var toDelete []pcommon.TraceID
	var droppedSpans, keptSpans int

	for tid, td := range p.traces {
		if now.Sub(td.arrivalAt) < p.cfg.DecisionWait {
			continue
		}
		if td.decision == decisionKeep {
			toKeep = append(toKeep, td.spans)
			keptSpans += td.spanCount
		} else {
			droppedSpans += td.spanCount
		}
		toDelete = append(toDelete, tid)
	}

	for _, tid := range toDelete {
		delete(p.traces, tid)
	}
	p.mu.Unlock()

	if droppedSpans > 0 || keptSpans > 0 {
		p.logger.Info("traceshrink sweep",
			zap.Int("kept_spans", keptSpans),
			zap.Int("dropped_spans", droppedSpans),
			zap.Int("traces_pending", len(p.traces)),
		)
	}

	for _, traces := range toKeep {
		if err := p.next.ConsumeTraces(context.Background(), traces); err != nil {
			p.logger.Error("failed to forward traces", zap.Error(err))
		}
	}
}

func (p *traceShrinkProcessor) flushAll() {
	p.mu.Lock()
	var toKeep []ptrace.Traces
	for _, td := range p.traces {
		if td.decision == decisionKeep {
			toKeep = append(toKeep, td.spans)
		}
	}
	p.traces = make(map[pcommon.TraceID]*traceData)
	p.mu.Unlock()

	for _, traces := range toKeep {
		if err := p.next.ConsumeTraces(context.Background(), traces); err != nil {
			p.logger.Error("failed to flush traces on shutdown", zap.Error(err))
		}
	}
}

func (p *traceShrinkProcessor) evictOldest() {
	var oldestID pcommon.TraceID
	var oldestTime time.Time
	first := true
	for tid, td := range p.traces {
		if first || td.arrivalAt.Before(oldestTime) {
			oldestID = tid
			oldestTime = td.arrivalAt
			first = false
		}
	}
	if !first {
		delete(p.traces, oldestID)
	}
}
