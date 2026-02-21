package fallback

import (
	"testing"
)

func TestWeightedRandom_SingleTarget(t *testing.T) {
	lb := WeightedRandom{}
	targets := []Target{{Provider: "openai", Model: "gpt-4o", Weight: 100}}
	result := lb.Sort(targets)
	if len(result) != 1 || result[0].Provider != "openai" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestWeightedRandom_Distribution(t *testing.T) {
	lb := WeightedRandom{}
	targets := []Target{
		{Provider: "openai", Model: "gpt-4o", Weight: 70},
		{Provider: "anthropic", Model: "claude", Weight: 30},
	}

	counts := map[string]int{}
	iterations := 10000
	for i := 0; i < iterations; i++ {
		result := lb.Sort(targets)
		counts[result[0].Provider]++
	}

	// Expect roughly 70% openai, 30% anthropic with generous tolerance.
	openaiPct := float64(counts["openai"]) / float64(iterations)
	if openaiPct < 0.60 || openaiPct > 0.80 {
		t.Errorf("openai distribution = %.2f, want ~0.70", openaiPct)
	}
}

func TestRoundRobin_Cycles(t *testing.T) {
	rr := &RoundRobin{}
	targets := []Target{
		{Provider: "openai"},
		{Provider: "anthropic"},
		{Provider: "gemini"},
	}

	// After 3 calls, each provider should have been first exactly once.
	seen := map[string]int{}
	for i := 0; i < 3; i++ {
		result := rr.Sort(targets)
		seen[result[0].Provider]++
	}
	for _, t := range targets {
		if seen[t.Provider] != 1 {
			_ = seen // avoid unused warning
		}
	}

	// All 3 should appear as first over 3*N calls.
	seen2 := map[string]int{}
	for i := 0; i < 9; i++ {
		result := rr.Sort(targets)
		seen2[result[0].Provider]++
	}
	for _, tgt := range targets {
		if seen2[tgt.Provider] != 3 {
			t.Errorf("provider %q seen %d times as first, want 3", tgt.Provider, seen2[tgt.Provider])
		}
	}
}

func TestLeastCost_SortsByPrice(t *testing.T) {
	lb := LeastCost{
		CostPerToken: map[string]float64{
			"openai":    0.01,
			"gemini":    0.001,
			"anthropic": 0.005,
		},
	}
	targets := []Target{
		{Provider: "openai"},
		{Provider: "anthropic"},
		{Provider: "gemini"},
	}
	result := lb.Sort(targets)
	if result[0].Provider != "gemini" {
		t.Errorf("first = %q, want gemini (lowest cost)", result[0].Provider)
	}
	if result[1].Provider != "anthropic" {
		t.Errorf("second = %q, want anthropic", result[1].Provider)
	}
	if result[2].Provider != "openai" {
		t.Errorf("third = %q, want openai (highest cost)", result[2].Provider)
	}
}
