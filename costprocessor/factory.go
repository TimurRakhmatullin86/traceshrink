package costprocessor

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
)

var Type = component.MustNewType("costprocessor")

func NewFactory() processor.Factory {
	return processor.NewFactory(
		Type,
		func() component.Config {
			return createDefaultConfig()
		},
		processor.WithTraces(createTracesProcessor, component.StabilityLevelAlpha),
	)
}

func createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Traces,
) (processor.Traces, error) {
	cCfg := cfg.(*Config)

	pt, err := BuiltinPricingTable()
	if err != nil {
		return nil, fmt.Errorf("failed to load builtin pricing table: %w", err)
	}

	return newCostProcessor(set.Logger, cCfg, pt, next), nil
}
