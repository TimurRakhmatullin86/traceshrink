package costprocessor

import (
	"encoding/json"
	"strings"
)

type ModelPricing struct {
	InputCostPerToken          float64 `json:"input_cost_per_token"`
	OutputCostPerToken         float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost    float64 `json:"cache_read_input_token_cost,omitempty"`
	OutputCostPerReasoningToken float64 `json:"output_cost_per_reasoning_token,omitempty"`
}

type PricingTable interface {
	Lookup(model string) (ModelPricing, bool)
}

type mapPricingTable struct {
	prices map[string]ModelPricing
}

func NewPricingTable(data []byte) (PricingTable, error) {
	var raw map[string]ModelPricing
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	normalized := make(map[string]ModelPricing, len(raw))
	for k, v := range raw {
		normalized[normalizeModel(k)] = v
	}
	return &mapPricingTable{prices: normalized}, nil
}

func (t *mapPricingTable) Lookup(model string) (ModelPricing, bool) {
	p, ok := t.prices[normalizeModel(model)]
	return p, ok
}

func normalizeModel(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}
