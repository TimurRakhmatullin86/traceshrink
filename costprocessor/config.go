package costprocessor

import "errors"

type Config struct {
	// ModelAttribute is the span attribute containing the LLM model name.
	ModelAttribute string `mapstructure:"model_attribute"`

	// InputTokensAttribute is the span attribute for input token count.
	InputTokensAttribute string `mapstructure:"input_tokens_attribute"`

	// OutputTokensAttribute is the span attribute for output token count.
	OutputTokensAttribute string `mapstructure:"output_tokens_attribute"`

	// CacheReadTokensAttribute is the span attribute for cache read token count.
	CacheReadTokensAttribute string `mapstructure:"cache_read_tokens_attribute"`

	// ReasoningTokensAttribute is the span attribute for reasoning/thinking token count.
	ReasoningTokensAttribute string `mapstructure:"reasoning_tokens_attribute"`

	// CostAttribute is the span attribute where the computed cost will be written.
	CostAttribute string `mapstructure:"cost_attribute"`

	// FallbackInputCostPerToken is used when the model is not in the pricing table.
	// Set to 0 to skip unknown models.
	FallbackInputCostPerToken float64 `mapstructure:"fallback_input_cost_per_token"`

	// FallbackOutputCostPerToken is used when the model is not in the pricing table.
	FallbackOutputCostPerToken float64 `mapstructure:"fallback_output_cost_per_token"`
}

func createDefaultConfig() *Config {
	return &Config{
		ModelAttribute:             "gen_ai.request.model",
		InputTokensAttribute:       "gen_ai.usage.input_tokens",
		OutputTokensAttribute:      "gen_ai.usage.output_tokens",
		CacheReadTokensAttribute:   "gen_ai.usage.cache_read_input_tokens",
		ReasoningTokensAttribute:   "gen_ai.usage.reasoning_tokens",
		CostAttribute:              "gen_ai.usage.cost_usd",
		FallbackInputCostPerToken:  0,
		FallbackOutputCostPerToken: 0,
	}
}

func (cfg *Config) Validate() error {
	if cfg.ModelAttribute == "" {
		return errors.New("model_attribute must not be empty")
	}
	if cfg.InputTokensAttribute == "" {
		return errors.New("input_tokens_attribute must not be empty")
	}
	if cfg.OutputTokensAttribute == "" {
		return errors.New("output_tokens_attribute must not be empty")
	}
	if cfg.CostAttribute == "" {
		return errors.New("cost_attribute must not be empty")
	}
	if cfg.FallbackInputCostPerToken < 0 {
		return errors.New("fallback_input_cost_per_token must be >= 0")
	}
	if cfg.FallbackOutputCostPerToken < 0 {
		return errors.New("fallback_output_cost_per_token must be >= 0")
	}
	return nil
}
