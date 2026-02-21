package mlrouter_test

import (
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/mlrouter"
)

func TestClassifier_Simple(t *testing.T) {
	c := mlrouter.NewClassifier()
	req := &types.ChatCompletionRequest{
		Model: "gpt-4o-mini",
		Messages: []types.Message{
			{Role: "user", Content: "What is the capital of France?"},
		},
	}
	f := c.Classify(req)
	if f.Tier != mlrouter.ComplexitySimple {
		t.Errorf("expected economy tier, got %q (score=%.2f)", f.Tier, f.ComplexityScore)
	}
}

func TestClassifier_Medium_WithCode(t *testing.T) {
	c := mlrouter.NewClassifier()
	req := &types.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []types.Message{
			{Role: "user", Content: "Write a Go function that sorts a slice:\n```go\nfunc sort(s []int) {}\n```"},
			{Role: "assistant", Content: "Here is the implementation..."},
			{Role: "user", Content: "Now add a concurrent version using goroutines and channels."},
		},
	}
	f := c.Classify(req)
	if f.Tier != mlrouter.ComplexityMedium && f.Tier != mlrouter.ComplexityComplex {
		t.Errorf("expected at least medium tier, got %q (score=%.2f)", f.Tier, f.ComplexityScore)
	}
	if !f.HasCodeBlocks {
		t.Error("expected HasCodeBlocks=true")
	}
}

func TestClassifier_Complex_LongTechnical(t *testing.T) {
	c := mlrouter.NewClassifier()
	longText := "Explain the differential calculus theorem and its application in neural network gradient descent optimization algorithms. " +
		"Include proof of convergence, runtime complexity analysis, and distributed gradient aggregation in Kubernetes-based training infrastructure. " +
		"Provide tensor decomposition examples and regression model evaluation metrics. " +
		"Also describe how async goroutines can parallelize the computation. " +
		"Finally, compare YAML configuration for distributed training vs JSON-based API configuration. " +
		"Repeat this paragraph to simulate a very long technical document. " +
		"Explain the differential calculus theorem and its application in neural network gradient descent optimization algorithms. " +
		"Include proof of convergence, runtime complexity analysis, and distributed gradient aggregation in Kubernetes-based training infrastructure. " +
		"Provide tensor decomposition examples and regression model evaluation metrics. "
	req := &types.ChatCompletionRequest{
		Model: "claude-opus-4-20250514",
		Messages: []types.Message{
			{Role: "user", Content: longText},
		},
	}
	f := c.Classify(req)
	if f.Tier != mlrouter.ComplexityComplex {
		t.Errorf("expected premium tier, got %q (score=%.2f)", f.Tier, f.ComplexityScore)
	}
}

func TestClassifier_ComplexityScoreRange(t *testing.T) {
	c := mlrouter.NewClassifier()
	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	f := c.Classify(req)
	if f.ComplexityScore < 0 || f.ComplexityScore > 1 {
		t.Errorf("complexity score %f out of [0,1]", f.ComplexityScore)
	}
}

func TestClassifier_Tiers_Exhaustive(t *testing.T) {
	c := mlrouter.NewClassifier()
	tests := []struct {
		content string
		wantMin string // must be at least this tier
	}{
		{"Hi", mlrouter.ComplexitySimple},
		{"Write code ```go\npackage main\n```", mlrouter.ComplexityMedium},
	}
	for _, tt := range tests {
		req := &types.ChatCompletionRequest{
			Messages: []types.Message{{Role: "user", Content: tt.content}},
		}
		f := c.Classify(req)
		if tt.wantMin == mlrouter.ComplexityComplex && f.Tier != mlrouter.ComplexityComplex {
			t.Errorf("content %q: expected premium, got %q", tt.content, f.Tier)
		}
	}
}
