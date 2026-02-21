package abtest

import (
	"fmt"
	"testing"
)

func TestAssignConsistency(t *testing.T) {
	exp := &Experiment{
		ID:     "exp-001",
		Status: StatusRunning,
		TrafficSplit: []TrafficSplit{
			{Variant: "control", Model: "openai/gpt-4o", Weight: 50},
			{Variant: "treatment", Model: "anthropic/claude-sonnet", Weight: 50},
		},
		Target: Target{SampleRate: 1.0},
	}

	// Same entity → same variant every time.
	v1 := Assign(exp, "user-abc")
	v2 := Assign(exp, "user-abc")
	if v1 != v2 {
		t.Errorf("inconsistent assignment for same entity: %q then %q", v1, v2)
	}
}

func TestAssignDistribution(t *testing.T) {
	exp := &Experiment{
		ID:     "exp-002",
		Status: StatusRunning,
		TrafficSplit: []TrafficSplit{
			{Variant: "control", Model: "openai/gpt-4o", Weight: 50},
			{Variant: "treatment", Model: "anthropic/claude-sonnet", Weight: 50},
		},
		Target: Target{SampleRate: 1.0},
	}

	counts := map[string]int{}
	n := 2000
	for i := 0; i < n; i++ {
		v := Assign(exp, fmt.Sprintf("entity-%d", i))
		counts[v]++
	}

	// Expect each variant to be within [40%, 60%] of total.
	for variant, c := range counts {
		pct := float64(c) / float64(n) * 100
		if pct < 40 || pct > 60 {
			t.Errorf("variant %q: expected ~50%%, got %.1f%%", variant, pct)
		}
	}
}

func TestAssignSampleRate(t *testing.T) {
	exp := &Experiment{
		ID:     "exp-003",
		Status: StatusRunning,
		TrafficSplit: []TrafficSplit{
			{Variant: "control", Model: "openai/gpt-4o", Weight: 100},
		},
		Target: Target{SampleRate: 0.1}, // only 10% participate
	}

	participating := 0
	n := 5000
	for i := 0; i < n; i++ {
		if Assign(exp, fmt.Sprintf("u-%d", i)) != "" {
			participating++
		}
	}

	pct := float64(participating) / float64(n) * 100
	if pct < 7 || pct > 13 {
		t.Errorf("sample rate 10%%: got %.1f%% participation", pct)
	}
}

func TestNormalTailP(t *testing.T) {
	// z = 1.96 → p ≈ 0.05 (two-tailed)
	p := normalTailP(1.96)
	if p < 0.04 || p > 0.06 {
		t.Errorf("normalTailP(1.96) = %.4f, want ~0.05", p)
	}
	// z = 2.576 → p ≈ 0.01
	p2 := normalTailP(2.576)
	if p2 < 0.009 || p2 > 0.011 {
		t.Errorf("normalTailP(2.576) = %.4f, want ~0.01", p2)
	}
}

func TestP95(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	p := P95(vals)
	if p < 9 || p > 10 {
		t.Errorf("P95 of 1-10 = %.1f, want ~9.5", p)
	}
}
