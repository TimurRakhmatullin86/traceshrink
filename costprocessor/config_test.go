package costprocessor

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	if cfg.ModelAttribute != "gen_ai.request.model" {
		t.Errorf("unexpected model_attribute: %s", cfg.ModelAttribute)
	}
	if cfg.CostAttribute != "gen_ai.usage.cost_usd" {
		t.Errorf("unexpected cost_attribute: %s", cfg.CostAttribute)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{"valid default", func(c *Config) {}, false},
		{"empty model_attribute", func(c *Config) { c.ModelAttribute = "" }, true},
		{"empty input_tokens_attribute", func(c *Config) { c.InputTokensAttribute = "" }, true},
		{"empty output_tokens_attribute", func(c *Config) { c.OutputTokensAttribute = "" }, true},
		{"empty cost_attribute", func(c *Config) { c.CostAttribute = "" }, true},
		{"negative fallback input", func(c *Config) { c.FallbackInputCostPerToken = -1 }, true},
		{"negative fallback output", func(c *Config) { c.FallbackOutputCostPerToken = -1 }, true},
		{"valid with fallback", func(c *Config) {
			c.FallbackInputCostPerToken = 1e-6
			c.FallbackOutputCostPerToken = 2e-6
		}, false},
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
