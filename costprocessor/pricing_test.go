package costprocessor

import (
	"math"
	"testing"
)

func TestNewPricingTable(t *testing.T) {
	data := []byte(`{
		"gpt-4o": {"input_cost_per_token": 2.5e-06, "output_cost_per_token": 1e-05},
		"claude-sonnet-4-20250514": {"input_cost_per_token": 3e-06, "output_cost_per_token": 1.5e-05, "cache_read_input_token_cost": 3e-07}
	}`)

	pt, err := NewPricingTable(data)
	if err != nil {
		t.Fatal(err)
	}

	p, ok := pt.Lookup("gpt-4o")
	if !ok {
		t.Fatal("gpt-4o not found")
	}
	if p.InputCostPerToken != 2.5e-06 {
		t.Errorf("expected 2.5e-06, got %e", p.InputCostPerToken)
	}

	p, ok = pt.Lookup("Claude-Sonnet-4-20250514")
	if !ok {
		t.Fatal("claude-sonnet-4 not found (case-insensitive)")
	}
	if p.CacheReadInputTokenCost != 3e-07 {
		t.Errorf("expected 3e-07 cache read cost, got %e", p.CacheReadInputTokenCost)
	}
}

func TestLookupUnknownModel(t *testing.T) {
	data := []byte(`{"gpt-4o": {"input_cost_per_token": 2.5e-06, "output_cost_per_token": 1e-05}}`)
	pt, _ := NewPricingTable(data)

	_, ok := pt.Lookup("nonexistent-model-xyz")
	if ok {
		t.Error("expected not found for unknown model")
	}
}

func TestEmptyTable(t *testing.T) {
	pt, err := NewPricingTable([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	_, ok := pt.Lookup("anything")
	if ok {
		t.Error("expected not found on empty table")
	}
}

func TestBuiltinPricingTable(t *testing.T) {
	pt, err := BuiltinPricingTable()
	if err != nil {
		t.Fatalf("builtin pricing table failed: %v", err)
	}

	known := []struct {
		model string
	}{
		{"gpt-4o"},
		{"gpt-4o-mini"},
	}
	for _, k := range known {
		p, ok := pt.Lookup(k.model)
		if !ok {
			t.Errorf("expected to find %s in builtin table", k.model)
			continue
		}
		if p.InputCostPerToken <= 0 || p.OutputCostPerToken <= 0 {
			t.Errorf("%s: expected positive costs, got in=%e out=%e", k.model, p.InputCostPerToken, p.OutputCostPerToken)
		}
	}
}

func TestCostCalculation(t *testing.T) {
	data := []byte(`{
		"gpt-4o": {
			"input_cost_per_token": 2.5e-06,
			"output_cost_per_token": 1e-05,
			"cache_read_input_token_cost": 1.25e-06,
			"output_cost_per_reasoning_token": 0
		}
	}`)
	pt, _ := NewPricingTable(data)
	p, _ := pt.Lookup("gpt-4o")

	inputTokens := int64(1000)
	outputTokens := int64(500)
	cost := float64(inputTokens)*p.InputCostPerToken + float64(outputTokens)*p.OutputCostPerToken
	expected := 1000*2.5e-06 + 500*1e-05
	if math.Abs(cost-expected) > 1e-12 {
		t.Errorf("cost = %e, expected %e", cost, expected)
	}
}
