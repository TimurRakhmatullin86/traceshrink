package traceshrink

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	if cfg.CostThreshold != 0.10 {
		t.Errorf("expected cost_threshold 0.10, got %f", cfg.CostThreshold)
	}
	if !cfg.KeepErrors {
		t.Error("expected keep_errors true")
	}
	if cfg.DurationThreshold != 5*time.Second {
		t.Errorf("expected duration_threshold 5s, got %v", cfg.DurationThreshold)
	}
	if cfg.CostAttribute != "llm.cost" {
		t.Errorf("expected cost_attribute 'llm.cost', got %q", cfg.CostAttribute)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid default",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:    "negative cost threshold",
			modify:  func(c *Config) { c.CostThreshold = -1 },
			wantErr: true,
		},
		{
			name:    "negative duration threshold",
			modify:  func(c *Config) { c.DurationThreshold = -1 },
			wantErr: true,
		},
		{
			name:    "zero decision wait",
			modify:  func(c *Config) { c.DecisionWait = 0 },
			wantErr: true,
		},
		{
			name:    "zero num traces",
			modify:  func(c *Config) { c.NumTraces = 0 },
			wantErr: true,
		},
		{
			name: "all filters disabled",
			modify: func(c *Config) {
				c.CostThreshold = 0
				c.KeepErrors = false
				c.DurationThreshold = 0
				c.KeepAttributes = nil
			},
			wantErr: true,
		},
		{
			name: "only keep_attributes enabled",
			modify: func(c *Config) {
				c.CostThreshold = 0
				c.KeepErrors = false
				c.DurationThreshold = 0
				c.KeepAttributes = []string{"important"}
			},
			wantErr: false,
		},
		{
			name: "only cost enabled",
			modify: func(c *Config) {
				c.KeepErrors = false
				c.DurationThreshold = 0
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := createDefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
