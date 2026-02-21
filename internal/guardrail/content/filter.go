package content

import (
	"context"
	"regexp"

	"github.com/llm-router/gateway/internal/guardrail"
)

// categoryPatterns maps content categories to detection patterns.
// These are intentionally broad; production deployments should use a classifier.
var categoryPatterns = map[string][]*regexp.Regexp{
	"hate": {
		regexp.MustCompile(`(?i)\b(hate\s+speech|racial\s+slur|ethnic\s+slur|bigot)\b`),
	},
	"violence": {
		regexp.MustCompile(`(?i)\b(how\s+to\s+(kill|murder|bomb|shoot)|build\s+a\s+bomb|make\s+a\s+weapon)\b`),
	},
	"sexual": {
		regexp.MustCompile(`(?i)\b(explicit\s+sexual|pornography|child\s+sexual)\b`),
	},
}

// Filter filters content by category using pattern matching.
type Filter struct {
	enabled    bool
	action     guardrail.Action
	categories map[string]bool
}

// NewFilter creates a content filter for the given categories.
func NewFilter(enabled bool, action guardrail.Action, categories []string) *Filter {
	cats := make(map[string]bool, len(categories))
	for _, c := range categories {
		cats[c] = true
	}
	return &Filter{enabled: enabled, action: action, categories: cats}
}

func (f *Filter) Name() string { return "content_filter" }

func (f *Filter) Check(_ context.Context, text string, _ guardrail.Direction) (*guardrail.Result, error) {
	if !f.enabled {
		return &guardrail.Result{}, nil
	}

	for category, patterns := range categoryPatterns {
		if !f.categories[category] {
			continue
		}
		for _, re := range patterns {
			if re.MatchString(text) {
				return &guardrail.Result{
					Triggered: true,
					Action:    f.action,
					Category:  category,
					Guardrail: "content_filter",
				}, nil
			}
		}
	}
	return &guardrail.Result{}, nil
}
