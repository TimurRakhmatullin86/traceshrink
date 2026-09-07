package costprocessor

import (
	"context"
	"math"
	"sync"
	"testing"

	"go.opentelemetry.io/collector/consumer"
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

func makePricingTable() PricingTable {
	data := []byte(`{
		"gpt-4o": {"input_cost_per_token": 2.5e-06, "output_cost_per_token": 1e-05},
		"claude-sonnet-4-20250514": {"input_cost_per_token": 3e-06, "output_cost_per_token": 1.5e-05, "cache_read_input_token_cost": 3e-07, "output_cost_per_reasoning_token": 0},
		"o1": {"input_cost_per_token": 1.5e-05, "output_cost_per_token": 6e-05, "output_cost_per_reasoning_token": 6e-05}
	}`)
	pt, _ := NewPricingTable(data)
	return pt
}

func makeSpan(td ptrace.Traces, model string, inputTokens, outputTokens int64) ptrace.Span {
	rs := td.ResourceSpans().AppendEmpty()
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("llm.chat")
	if model != "" {
		span.Attributes().PutStr("gen_ai.request.model", model)
	}
	if inputTokens > 0 {
		span.Attributes().PutInt("gen_ai.usage.input_tokens", inputTokens)
	}
	if outputTokens > 0 {
		span.Attributes().PutInt("gen_ai.usage.output_tokens", outputTokens)
	}
	return span
}

func getSpanCost(td ptrace.Traces) (float64, bool) {
	span := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	v, ok := span.Attributes().Get("gen_ai.usage.cost_usd")
	if !ok {
		return 0, false
	}
	return v.Double(), true
}

func TestAnnotateKnownModel(t *testing.T) {
	cfg := createDefaultConfig()
	mc := &mockConsumer{}
	p := newCostProcessor(zap.NewNop(), cfg, makePricingTable(), mc)

	td := ptrace.NewTraces()
	makeSpan(td, "gpt-4o", 1000, 500)

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	expected := 1000*2.5e-06 + 500*1e-05
	cost, ok := getSpanCost(td)
	if !ok {
		t.Fatal("cost attribute not set")
	}
	if math.Abs(cost-expected) > 1e-12 {
		t.Errorf("cost = %e, expected %e", cost, expected)
	}
}

func TestAnnotateWithCacheReadTokens(t *testing.T) {
	cfg := createDefaultConfig()
	mc := &mockConsumer{}
	p := newCostProcessor(zap.NewNop(), cfg, makePricingTable(), mc)

	td := ptrace.NewTraces()
	span := makeSpan(td, "claude-sonnet-4-20250514", 1000, 500)
	span.Attributes().PutInt("gen_ai.usage.cache_read_input_tokens", 200)

	_ = p.ConsumeTraces(context.Background(), td)

	// cost = 1000*3e-6 + 500*1.5e-5 - 200*3e-6 + 200*3e-7
	// = 0.003 + 0.0075 - 0.0006 + 0.00006 = 0.00996
	expected := 1000*3e-06 + 500*1.5e-05 - 200*3e-06 + 200*3e-07
	cost, ok := getSpanCost(td)
	if !ok {
		t.Fatal("cost attribute not set")
	}
	if math.Abs(cost-expected) > 1e-12 {
		t.Errorf("cost = %e, expected %e", cost, expected)
	}
}

func TestAnnotateWithReasoningTokens(t *testing.T) {
	cfg := createDefaultConfig()
	mc := &mockConsumer{}
	p := newCostProcessor(zap.NewNop(), cfg, makePricingTable(), mc)

	td := ptrace.NewTraces()
	span := makeSpan(td, "o1", 1000, 500)
	span.Attributes().PutInt("gen_ai.usage.reasoning_tokens", 300)

	_ = p.ConsumeTraces(context.Background(), td)

	// cost = 1000*1.5e-5 + 500*6e-5 - 300*6e-5 + 300*6e-5
	// reasoning cost == output cost for o1, so net zero adjustment
	expected := 1000*1.5e-05 + 500*6e-05
	cost, ok := getSpanCost(td)
	if !ok {
		t.Fatal("cost attribute not set")
	}
	if math.Abs(cost-expected) > 1e-12 {
		t.Errorf("cost = %e, expected %e", cost, expected)
	}
}

func TestUnknownModelNoFallback(t *testing.T) {
	cfg := createDefaultConfig()
	mc := &mockConsumer{}
	p := newCostProcessor(zap.NewNop(), cfg, makePricingTable(), mc)

	td := ptrace.NewTraces()
	makeSpan(td, "unknown-model-xyz", 1000, 500)

	_ = p.ConsumeTraces(context.Background(), td)

	_, ok := getSpanCost(td)
	if ok {
		t.Error("expected no cost attribute for unknown model without fallback")
	}
}

func TestUnknownModelWithFallback(t *testing.T) {
	cfg := createDefaultConfig()
	cfg.FallbackInputCostPerToken = 1e-06
	cfg.FallbackOutputCostPerToken = 2e-06
	mc := &mockConsumer{}
	p := newCostProcessor(zap.NewNop(), cfg, makePricingTable(), mc)

	td := ptrace.NewTraces()
	makeSpan(td, "unknown-model-xyz", 1000, 500)

	_ = p.ConsumeTraces(context.Background(), td)

	expected := 1000*1e-06 + 500*2e-06
	cost, ok := getSpanCost(td)
	if !ok {
		t.Fatal("expected cost attribute with fallback pricing")
	}
	if math.Abs(cost-expected) > 1e-12 {
		t.Errorf("cost = %e, expected %e", cost, expected)
	}
}

func TestMissingTokensSkip(t *testing.T) {
	cfg := createDefaultConfig()
	mc := &mockConsumer{}
	p := newCostProcessor(zap.NewNop(), cfg, makePricingTable(), mc)

	td := ptrace.NewTraces()
	makeSpan(td, "gpt-4o", 0, 0)

	_ = p.ConsumeTraces(context.Background(), td)

	_, ok := getSpanCost(td)
	if ok {
		t.Error("expected no cost attribute when tokens are 0")
	}
}

func TestMissingModelSkip(t *testing.T) {
	cfg := createDefaultConfig()
	mc := &mockConsumer{}
	p := newCostProcessor(zap.NewNop(), cfg, makePricingTable(), mc)

	td := ptrace.NewTraces()
	makeSpan(td, "", 1000, 500)

	_ = p.ConsumeTraces(context.Background(), td)

	_, ok := getSpanCost(td)
	if ok {
		t.Error("expected no cost attribute when model is missing")
	}
}

func TestForwardsToNext(t *testing.T) {
	cfg := createDefaultConfig()
	mc := &mockConsumer{}
	p := newCostProcessor(zap.NewNop(), cfg, makePricingTable(), mc)

	td := ptrace.NewTraces()
	makeSpan(td, "gpt-4o", 100, 50)

	_ = p.ConsumeTraces(context.Background(), td)

	if len(mc.traces) != 1 {
		t.Errorf("expected 1 forwarded batch, got %d", len(mc.traces))
	}
}

func BenchmarkAnnotateSpan(b *testing.B) {
	cfg := createDefaultConfig()
	mc := &mockConsumer{}
	p := newCostProcessor(zap.NewNop(), cfg, makePricingTable(), mc)

	td := ptrace.NewTraces()
	makeSpan(td, "gpt-4o", 1000, 500)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = p.ConsumeTraces(context.Background(), td)
	}
}
