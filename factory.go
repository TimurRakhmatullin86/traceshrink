package traceshrink

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
)

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
	tCfg := cfg.(*Config)
	return newTraceShrinkProcessor(set.Logger, tCfg, next), nil
}
