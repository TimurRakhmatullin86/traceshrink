package traceshrink

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type nopConsumer struct{}

func (n *nopConsumer) ConsumeTraces(_ context.Context, _ ptrace.Traces) error { return nil }
func (n *nopConsumer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func generateBatch(batchSize int, costRate, errorRate float64) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "bench-svc")
	ils := rs.ScopeSpans().AppendEmpty()
	ils.Scope().SetName("bench")

	rng := rand.New(rand.NewSource(42))
	for i := 0; i < batchSize; i++ {
		span := ils.Spans().AppendEmpty()
		traceID := [16]byte{byte(i >> 8), byte(i)}
		span.SetTraceID(pcommon.TraceID(traceID))
		span.SetSpanID(pcommon.SpanID([8]byte{byte(i)}))
		span.SetName("op")
		span.SetStartTimestamp(pcommon.Timestamp(1_000_000_000))
		span.SetEndTimestamp(pcommon.Timestamp(1_100_000_000))

		cost := rng.Float64() * 0.05
		if rng.Float64() < costRate {
			cost = 0.10 + rng.Float64()*2.0
		}
		span.Attributes().PutDouble("llm.cost", cost)

		if rng.Float64() < errorRate {
			span.Status().SetCode(ptrace.StatusCodeError)
		} else {
			span.Status().SetCode(ptrace.StatusCodeOk)
		}
	}
	return td
}

func BenchmarkConsumeTraces_1K(b *testing.B) {
	cfg := createDefaultConfig()
	cfg.DecisionWait = 1 * time.Hour
	cfg.NumTraces = 1_000_000
	p := newTraceShrinkProcessor(zap.NewNop(), cfg, &nopConsumer{})

	batch := generateBatch(1000, 0.008, 0.002)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = p.ConsumeTraces(context.Background(), batch)
	}
}

func BenchmarkConsumeTraces_10K(b *testing.B) {
	cfg := createDefaultConfig()
	cfg.DecisionWait = 1 * time.Hour
	cfg.NumTraces = 1_000_000
	p := newTraceShrinkProcessor(zap.NewNop(), cfg, &nopConsumer{})

	batch := generateBatch(10000, 0.008, 0.002)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = p.ConsumeTraces(context.Background(), batch)
	}
}

func BenchmarkEvaluateDecision(b *testing.B) {
	cfg := createDefaultConfig()
	p := newTraceShrinkProcessor(zap.NewNop(), cfg, &nopConsumer{})

	span := ptrace.NewSpan()
	span.Attributes().PutDouble("llm.cost", 0.05)
	span.Status().SetCode(ptrace.StatusCodeOk)
	span.SetStartTimestamp(pcommon.Timestamp(1_000_000_000))
	span.SetEndTimestamp(pcommon.Timestamp(1_100_000_000))

	td := &traceData{
		totalCost:  0.05,
		hasError:   false,
		maxDurNano: 100_000_000,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p.evaluate(td, span)
	}
}
