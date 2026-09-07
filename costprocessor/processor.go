package costprocessor

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type costProcessor struct {
	logger  *zap.Logger
	cfg     *Config
	pricing PricingTable
	next    consumer.Traces
}

func newCostProcessor(logger *zap.Logger, cfg *Config, pricing PricingTable, next consumer.Traces) *costProcessor {
	return &costProcessor{
		logger:  logger,
		cfg:     cfg,
		pricing: pricing,
		next:    next,
	}
}

func (p *costProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

func (p *costProcessor) Start(_ context.Context, _ component.Host) error {
	return nil
}

func (p *costProcessor) Shutdown(_ context.Context) error {
	return nil
}

func (p *costProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		ilss := rss.At(i).ScopeSpans()
		for j := 0; j < ilss.Len(); j++ {
			spans := ilss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				p.annotateSpan(spans.At(k))
			}
		}
	}
	return p.next.ConsumeTraces(ctx, td)
}

func (p *costProcessor) annotateSpan(span ptrace.Span) {
	attrs := span.Attributes()

	modelVal, ok := attrs.Get(p.cfg.ModelAttribute)
	if !ok {
		return
	}
	model := modelVal.Str()
	if model == "" {
		return
	}

	inputTokens := getInt64Attr(attrs, p.cfg.InputTokensAttribute)
	outputTokens := getInt64Attr(attrs, p.cfg.OutputTokensAttribute)
	if inputTokens == 0 && outputTokens == 0 {
		return
	}

	pricing, found := p.pricing.Lookup(model)
	if !found {
		if p.cfg.FallbackInputCostPerToken == 0 && p.cfg.FallbackOutputCostPerToken == 0 {
			return
		}
		pricing = ModelPricing{
			InputCostPerToken:  p.cfg.FallbackInputCostPerToken,
			OutputCostPerToken: p.cfg.FallbackOutputCostPerToken,
		}
	}

	cost := float64(inputTokens) * pricing.InputCostPerToken
	cost += float64(outputTokens) * pricing.OutputCostPerToken

	cacheReadTokens := getInt64Attr(attrs, p.cfg.CacheReadTokensAttribute)
	if cacheReadTokens > 0 && pricing.CacheReadInputTokenCost > 0 {
		cost -= float64(cacheReadTokens) * pricing.InputCostPerToken
		cost += float64(cacheReadTokens) * pricing.CacheReadInputTokenCost
	}

	reasoningTokens := getInt64Attr(attrs, p.cfg.ReasoningTokensAttribute)
	if reasoningTokens > 0 && pricing.OutputCostPerReasoningToken > 0 {
		cost -= float64(reasoningTokens) * pricing.OutputCostPerToken
		cost += float64(reasoningTokens) * pricing.OutputCostPerReasoningToken
	}

	attrs.PutDouble(p.cfg.CostAttribute, cost)
}

func getInt64Attr(attrs pcommon.Map, key string) int64 {
	if key == "" {
		return 0
	}
	v, ok := attrs.Get(key)
	if !ok {
		return 0
	}
	switch v.Type() {
	case pcommon.ValueTypeInt:
		return v.Int()
	case pcommon.ValueTypeDouble:
		return int64(v.Double())
	default:
		return 0
	}
}
