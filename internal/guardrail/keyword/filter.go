package keyword

import (
	"context"
	"strings"

	"github.com/llm-router/gateway/internal/guardrail"
)

// Filter blocks or logs requests containing forbidden keywords.
type Filter struct {
	enabled  bool
	action   guardrail.Action
	keywords []string // lowercased for case-insensitive matching
}

// NewFilter creates a keyword filter.
func NewFilter(enabled bool, action guardrail.Action, keywords []string) *Filter {
	lower := make([]string, len(keywords))
	for i, k := range keywords {
		lower[i] = strings.ToLower(k)
	}
	return &Filter{enabled: enabled, action: action, keywords: lower}
}

func (f *Filter) Name() string { return "custom_keywords" }

func (f *Filter) Check(_ context.Context, text string, _ guardrail.Direction) (*guardrail.Result, error) {
	if !f.enabled || len(f.keywords) == 0 {
		return &guardrail.Result{}, nil
	}

	lower := strings.ToLower(text)
	for _, kw := range f.keywords {
		if strings.Contains(lower, kw) {
			return &guardrail.Result{
				Triggered: true,
				Action:    f.action,
				Category:  kw,
				Guardrail: "custom_keywords",
			}, nil
		}
	}
	return &guardrail.Result{}, nil
}
