package traceshrink

import (
	"context"
	"fmt"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type countingConsumer struct {
	spans    atomic.Int64
	traces   atomic.Int64
	totalCost atomic.Int64 // cost * 1000 (millicents)
}

func (c *countingConsumer) ConsumeTraces(_ context.Context, td ptrace.Traces) error {
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		ilss := rss.At(i).ScopeSpans()
		for j := 0; j < ilss.Len(); j++ {
			spans := ilss.At(j).Spans()
			c.spans.Add(int64(spans.Len()))
			for k := 0; k < spans.Len(); k++ {
				v, ok := spans.At(k).Attributes().Get("llm.cost")
				if ok {
					c.totalCost.Add(int64(v.Double() * 1000))
				}
			}
		}
	}
	c.traces.Add(1)
	return nil
}

func (c *countingConsumer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func TestSimulation1MSpans(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 1M span simulation in short mode")
	}

	totalSpans := 1_000_000
	spansPerTrace := 5
	totalTraces := totalSpans / spansPerTrace

	// Realistic distribution for AI/LLM workload:
	// - 0.8% of traces have cost > $0.10 (expensive LLM calls)
	// - 0.2% of traces have errors
	// - 1% of traces are slow (> 5s)
	// - Rest are cheap, fast, healthy
	expensiveRate := 0.008
	errorRate := 0.002
	slowRate := 0.01

	cfg := createDefaultConfig()
	cfg.DecisionWait = 100 * time.Millisecond
	cfg.NumTraces = uint64(totalTraces + 1000)

	cc := &countingConsumer{}
	p := newTraceShrinkProcessor(zap.NewNop(), cfg, cc)

	rng := rand.New(rand.NewSource(2026))
	var inputCostMillicents int64
	var expectedKeep int

	start := time.Now()

	for i := 0; i < totalTraces; i++ {
		traceID := [16]byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)}
		td := ptrace.NewTraces()
		rs := td.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", "ai-gateway")
		ils := rs.ScopeSpans().AppendEmpty()

		isExpensive := rng.Float64() < expensiveRate
		isError := rng.Float64() < errorRate
		isSlow := rng.Float64() < slowRate
		shouldKeep := isExpensive || isError || isSlow

		if shouldKeep {
			expectedKeep++
		}

		for j := 0; j < spansPerTrace; j++ {
			span := ils.Spans().AppendEmpty()
			span.SetTraceID(pcommon.TraceID(traceID))
			span.SetSpanID(pcommon.SpanID([8]byte{byte(j)}))
			span.SetName("llm.chat.completion")

			var cost float64
			if isExpensive {
				cost = 0.10 + rng.Float64()*5.0
			} else {
				cost = rng.Float64() * 0.02
			}
			span.Attributes().PutDouble("llm.cost", cost)
			inputCostMillicents += int64(cost * 1000)

			if isSlow {
				span.SetStartTimestamp(pcommon.Timestamp(1_000_000_000))
				span.SetEndTimestamp(pcommon.Timestamp(1_000_000_000 + uint64((6+rng.Intn(30))*int(time.Second))))
			} else {
				span.SetStartTimestamp(pcommon.Timestamp(1_000_000_000))
				span.SetEndTimestamp(pcommon.Timestamp(1_000_000_000 + uint64(rng.Intn(int(4*time.Second)))))
			}

			if isError && j == spansPerTrace-1 {
				span.Status().SetCode(ptrace.StatusCodeError)
				span.Status().SetMessage("upstream timeout")
			} else {
				span.Status().SetCode(ptrace.StatusCodeOk)
			}
		}

		_ = p.ConsumeTraces(context.Background(), td)
	}

	ingestDuration := time.Since(start)

	time.Sleep(200 * time.Millisecond)
	p.sweep()

	keptSpans := cc.spans.Load()
	keptCostMC := cc.totalCost.Load()

	retentionPct := float64(keptSpans) / float64(totalSpans) * 100
	dropPct := 100 - retentionPct
	costRetainedPct := float64(keptCostMC) / float64(inputCostMillicents) * 100

	fmt.Printf("\n=== TraceShrink Simulation Results ===\n")
	fmt.Printf("Input:           %d spans (%d traces)\n", totalSpans, totalTraces)
	fmt.Printf("Output:          %d spans (%.1f%% retained)\n", keptSpans, retentionPct)
	fmt.Printf("Dropped:         %.1f%%\n", dropPct)
	fmt.Printf("Input cost:      $%.2f\n", float64(inputCostMillicents)/1000)
	fmt.Printf("Cost retained:   $%.2f (%.1f%% of total cost)\n", float64(keptCostMC)/1000, costRetainedPct)
	fmt.Printf("Expected keep:   ~%d traces (~%d spans)\n", expectedKeep, expectedKeep*spansPerTrace)
	fmt.Printf("Ingestion time:  %v (%.0f spans/sec)\n", ingestDuration, float64(totalSpans)/ingestDuration.Seconds())
	fmt.Printf("Per-span:        %v\n", ingestDuration/time.Duration(totalSpans))
	fmt.Printf("=====================================\n\n")

	if retentionPct > 5 {
		t.Errorf("retention too high: %.1f%% (expected <5%%)", retentionPct)
	}
	if costRetainedPct < 60 {
		t.Errorf("cost retention too low: %.1f%% (expected >60%%)", costRetainedPct)
	}
}
