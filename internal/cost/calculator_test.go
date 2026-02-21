package cost_test

import (
	"testing"

	"github.com/llm-router/gateway/internal/cost"
)

func TestCalculate(t *testing.T) {
	models := map[string]cost.ModelPricing{
		"gpt-4o": {
			InputPerMillionTokens:  2.50,
			OutputPerMillionTokens: 10.00,
		},
		"claude-sonnet-4-5": {
			InputPerMillionTokens:  3.00,
			OutputPerMillionTokens: 15.00,
		},
	}
	calc := cost.NewCalculator(models)

	tests := []struct {
		name             string
		model            string
		promptTokens     int
		completionTokens int
		wantCost         float64
	}{
		{
			name:             "gpt-4o basic",
			model:            "gpt-4o",
			promptTokens:     1_000_000,
			completionTokens: 0,
			wantCost:         2.50,
		},
		{
			name:             "gpt-4o output",
			model:            "gpt-4o",
			promptTokens:     0,
			completionTokens: 1_000_000,
			wantCost:         10.00,
		},
		{
			name:             "gpt-4o combined",
			model:            "gpt-4o",
			promptTokens:     500_000,
			completionTokens: 100_000,
			wantCost:         1.25 + 1.00, // 0.5M*2.50/M + 0.1M*10.00/M
		},
		{
			name:             "provider prefix stripped",
			model:            "openai/gpt-4o",
			promptTokens:     1_000_000,
			completionTokens: 0,
			wantCost:         2.50,
		},
		{
			name:             "unknown model returns 0",
			model:            "unknown-model",
			promptTokens:     1_000_000,
			completionTokens: 1_000_000,
			wantCost:         0,
		},
		{
			name:             "zero tokens",
			model:            "gpt-4o",
			promptTokens:     0,
			completionTokens: 0,
			wantCost:         0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calc.Calculate(tt.model, tt.promptTokens, tt.completionTokens)
			if got != tt.wantCost {
				t.Errorf("Calculate(%q, %d, %d) = %f, want %f",
					tt.model, tt.promptTokens, tt.completionTokens, got, tt.wantCost)
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text      string
		wantAtLeast int
	}{
		{"Hello, world!", 2},
		{"", 1}, // minimum 1
		{"This is a longer text that should have more tokens than a short one", 10},
	}
	for _, tt := range tests {
		got := cost.EstimateTokens(tt.text)
		if got < tt.wantAtLeast {
			t.Errorf("EstimateTokens(%q) = %d, want >= %d", tt.text, got, tt.wantAtLeast)
		}
	}
}
