package traceshrink

import (
	"errors"
	"time"
)

type Config struct {
	// CostThreshold: keep traces where any span has a cost attribute >= this value (USD).
	// Set to 0 to disable cost-based filtering.
	CostThreshold float64 `mapstructure:"cost_threshold"`

	// CostAttribute is the span attribute name that holds the cost value.
	// Defaults to "gen_ai.usage.cost_usd" if empty.
	CostAttribute string `mapstructure:"cost_attribute"`

	// KeepErrors: if true, always keep traces that contain at least one span with status ERROR.
	KeepErrors bool `mapstructure:"keep_errors"`

	// DurationThreshold: keep traces where total duration exceeds this value.
	// Set to 0 to disable duration-based filtering.
	DurationThreshold time.Duration `mapstructure:"duration_threshold"`

	// KeepAttributes: keep traces where any span has one of these attributes set (any value).
	// Useful for custom business-logic markers like "important", "billable", etc.
	KeepAttributes []string `mapstructure:"keep_attributes"`

	// DecisionWait is how long to wait for a complete trace before making a sampling decision.
	// Longer values use more memory but catch more spans belonging to the same trace.
	DecisionWait time.Duration `mapstructure:"decision_wait"`

	// NumTraces is the maximum number of traces kept in memory awaiting a decision.
	NumTraces uint64 `mapstructure:"num_traces"`
}

func createDefaultConfig() *Config {
	return &Config{
		CostThreshold:     0.10,
		CostAttribute:     "gen_ai.usage.cost_usd",
		KeepErrors:        true,
		DurationThreshold: 5 * time.Second,
		KeepAttributes:    nil,
		DecisionWait:      30 * time.Second,
		NumTraces:         100_000,
	}
}

func (cfg *Config) Validate() error {
	if cfg.CostThreshold < 0 {
		return errors.New("cost_threshold must be >= 0")
	}
	if cfg.DurationThreshold < 0 {
		return errors.New("duration_threshold must be >= 0")
	}
	if cfg.DecisionWait <= 0 {
		return errors.New("decision_wait must be > 0")
	}
	if cfg.NumTraces == 0 {
		return errors.New("num_traces must be > 0")
	}
	if !cfg.KeepErrors && cfg.CostThreshold == 0 && cfg.DurationThreshold == 0 && len(cfg.KeepAttributes) == 0 {
		return errors.New("at least one filter must be enabled: cost_threshold > 0, keep_errors: true, duration_threshold > 0, or keep_attributes non-empty")
	}
	return nil
}
