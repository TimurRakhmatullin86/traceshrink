package traceshrink

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type mockConsumer struct {
	mu     sync.Mutex
	traces []ptrace.Traces
}

func (m *mockConsumer) ConsumeTraces(_ context.Context, td ptrace.Traces) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.traces = append(m.traces, td)
	return nil
}

func (m *mockConsumer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func (m *mockConsumer) totalSpans() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for _, td := range m.traces {
		rss := td.ResourceSpans()
		for i := 0; i < rss.Len(); i++ {
			ilss := rss.At(i).ScopeSpans()
			for j := 0; j < ilss.Len(); j++ {
				total += ilss.At(j).Spans().Len()
			}
		}
	}
	return total
}

func newTestTraces(traceID [16]byte, spanCount int, opts ...spanOption) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-svc")
	ils := rs.ScopeSpans().AppendEmpty()
	ils.Scope().SetName("test")

	for i := 0; i < spanCount; i++ {
		span := ils.Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID(traceID))
		span.SetSpanID(pcommon.SpanID([8]byte{byte(i + 1)}))
		span.SetName("test-span")
		span.SetStartTimestamp(pcommon.Timestamp(1000000000))
		span.SetEndTimestamp(pcommon.Timestamp(1100000000))
		span.Status().SetCode(ptrace.StatusCodeOk)
		for _, opt := range opts {
			opt(span)
		}
	}
	return td
}

type spanOption func(ptrace.Span)

func withCost(cost float64) spanOption {
	return func(s ptrace.Span) {
		s.Attributes().PutDouble("llm.cost", cost)
	}
}

func withError() spanOption {
	return func(s ptrace.Span) {
		s.Status().SetCode(ptrace.StatusCodeError)
		s.Status().SetMessage("something failed")
	}
}

func withDuration(d time.Duration) spanOption {
	return func(s ptrace.Span) {
		s.SetStartTimestamp(pcommon.Timestamp(1000000000))
		s.SetEndTimestamp(pcommon.Timestamp(1000000000 + uint64(d.Nanoseconds())))
	}
}

func withAttribute(key, val string) spanOption {
	return func(s ptrace.Span) {
		s.Attributes().PutStr(key, val)
	}
}

func newTestProcessor(cfg *Config) (*traceShrinkProcessor, *mockConsumer) {
	mc := &mockConsumer{}
	p := newTraceShrinkProcessor(zap.NewNop(), cfg, mc)
	return p, mc
}

func TestKeepExpensiveTrace(t *testing.T) {
	cfg := createDefaultConfig()
	cfg.DecisionWait = 100 * time.Millisecond
	p, _ := newTestProcessor(cfg)

	traceID := [16]byte{1}
	td := newTestTraces(traceID, 5, withCost(0.50))

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	p.mu.Lock()
	tr := p.traces[pcommon.TraceID(traceID)]
	if tr.decision != decisionKeep {
		t.Errorf("expected decisionKeep for expensive trace, got %d", tr.decision)
	}
	p.mu.Unlock()
}

func TestDropCheapTrace(t *testing.T) {
	cfg := createDefaultConfig()
	cfg.DecisionWait = 100 * time.Millisecond
	cfg.KeepErrors = false
	cfg.DurationThreshold = 0
	p, _ := newTestProcessor(cfg)

	traceID := [16]byte{2}
	td := newTestTraces(traceID, 5, withCost(0.01))

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	p.mu.Lock()
	tr := p.traces[pcommon.TraceID(traceID)]
	if tr.decision != decisionPending {
		t.Errorf("expected decisionPending for cheap trace, got %d", tr.decision)
	}
	p.mu.Unlock()
}

func TestKeepErrorTrace(t *testing.T) {
	cfg := createDefaultConfig()
	cfg.DecisionWait = 100 * time.Millisecond
	cfg.CostThreshold = 0
	cfg.DurationThreshold = 0
	p, _ := newTestProcessor(cfg)

	traceID := [16]byte{3}
	td := newTestTraces(traceID, 1, withError())

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	p.mu.Lock()
	tr := p.traces[pcommon.TraceID(traceID)]
	if tr.decision != decisionKeep {
		t.Errorf("expected decisionKeep for error trace, got %d", tr.decision)
	}
	p.mu.Unlock()
}

func TestKeepSlowTrace(t *testing.T) {
	cfg := createDefaultConfig()
	cfg.DecisionWait = 100 * time.Millisecond
	cfg.CostThreshold = 0
	cfg.KeepErrors = false
	cfg.DurationThreshold = 2 * time.Second
	p, _ := newTestProcessor(cfg)

	traceID := [16]byte{4}
	td := newTestTraces(traceID, 1, withDuration(10*time.Second))

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	p.mu.Lock()
	tr := p.traces[pcommon.TraceID(traceID)]
	if tr.decision != decisionKeep {
		t.Errorf("expected decisionKeep for slow trace, got %d", tr.decision)
	}
	p.mu.Unlock()
}

func TestKeepTraceWithMarkerAttribute(t *testing.T) {
	cfg := createDefaultConfig()
	cfg.DecisionWait = 100 * time.Millisecond
	cfg.CostThreshold = 0
	cfg.KeepErrors = false
	cfg.DurationThreshold = 0
	cfg.KeepAttributes = []string{"important"}
	p, _ := newTestProcessor(cfg)

	traceID := [16]byte{5}
	td := newTestTraces(traceID, 1, withAttribute("important", "true"))

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	p.mu.Lock()
	tr := p.traces[pcommon.TraceID(traceID)]
	if tr.decision != decisionKeep {
		t.Errorf("expected decisionKeep for attributed trace, got %d", tr.decision)
	}
	p.mu.Unlock()
}

func TestSweepForwardsKeptAndDropsPending(t *testing.T) {
	cfg := createDefaultConfig()
	cfg.DecisionWait = 50 * time.Millisecond
	cfg.KeepErrors = false
	cfg.DurationThreshold = 0
	p, mc := newTestProcessor(cfg)

	expensive := [16]byte{10}
	cheap := [16]byte{11}
	tdExpensive := newTestTraces(expensive, 3, withCost(1.0))
	tdCheap := newTestTraces(cheap, 7, withCost(0.001))

	_ = p.ConsumeTraces(context.Background(), tdExpensive)
	_ = p.ConsumeTraces(context.Background(), tdCheap)

	time.Sleep(100 * time.Millisecond)
	p.sweep()

	forwarded := mc.totalSpans()
	if forwarded != 3 {
		t.Errorf("expected 3 forwarded spans (expensive trace), got %d", forwarded)
	}

	p.mu.Lock()
	remaining := len(p.traces)
	p.mu.Unlock()
	if remaining != 0 {
		t.Errorf("expected 0 remaining traces after sweep, got %d", remaining)
	}
}

func TestExtractCostFromDifferentTypes(t *testing.T) {
	cfg := createDefaultConfig()
	p, _ := newTestProcessor(cfg)

	tests := []struct {
		name     string
		setAttr  func(ptrace.Span)
		expected float64
	}{
		{
			name: "double",
			setAttr: func(s ptrace.Span) {
				s.Attributes().PutDouble("llm.cost", 0.42)
			},
			expected: 0.42,
		},
		{
			name: "int",
			setAttr: func(s ptrace.Span) {
				s.Attributes().PutInt("llm.cost", 5)
			},
			expected: 5.0,
		},
		{
			name: "string",
			setAttr: func(s ptrace.Span) {
				s.Attributes().PutStr("llm.cost", "0.99")
			},
			expected: 0.99,
		},
		{
			name: "missing",
			setAttr: func(s ptrace.Span) {
			},
			expected: 0,
		},
		{
			name: "invalid string",
			setAttr: func(s ptrace.Span) {
				s.Attributes().PutStr("llm.cost", "not-a-number")
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			tt.setAttr(span)
			got := p.extractCost(span)
			if got != tt.expected {
				t.Errorf("extractCost() = %f, want %f", got, tt.expected)
			}
		})
	}
}

func TestEvictsOldestWhenFull(t *testing.T) {
	cfg := createDefaultConfig()
	cfg.NumTraces = 2
	cfg.DecisionWait = 10 * time.Second
	p, _ := newTestProcessor(cfg)

	for i := byte(1); i <= 3; i++ {
		td := newTestTraces([16]byte{i}, 1, withCost(0.001))
		_ = p.ConsumeTraces(context.Background(), td)
	}

	p.mu.Lock()
	count := len(p.traces)
	p.mu.Unlock()
	if count > 2 {
		t.Errorf("expected at most 2 traces in memory, got %d", count)
	}
}

func TestFactoryCreatesProcessor(t *testing.T) {
	f := NewFactory()
	if f.Type() != Type {
		t.Errorf("expected type %v, got %v", Type, f.Type())
	}
}
