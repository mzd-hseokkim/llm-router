package pii

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/llm-router/gateway/internal/guardrail"
)

var patterns = map[string]*regexp.Regexp{
	"credit_card": regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`),
	"ssn":         regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
	"email":       regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
	"phone_us":    regexp.MustCompile(`\b(?:\+1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b`),
	"ip_address":  regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
	"korean_rrn":  regexp.MustCompile(`\b\d{6}-[1-4]\d{6}\b`),
}

// Detector detects and optionally masks PII in text.
type Detector struct {
	enabled    bool
	action     guardrail.Action
	categories map[string]bool
}

// NewDetector creates a PII detector. If categories is empty, all are enabled.
func NewDetector(enabled bool, action guardrail.Action, categories []string) *Detector {
	cats := make(map[string]bool)
	if len(categories) == 0 {
		for k := range patterns {
			cats[k] = true
		}
	} else {
		for _, c := range categories {
			cats[c] = true
		}
	}
	return &Detector{enabled: enabled, action: action, categories: cats}
}

func (d *Detector) Name() string { return "pii" }

func (d *Detector) Check(_ context.Context, text string, _ guardrail.Direction) (*guardrail.Result, error) {
	if !d.enabled {
		return &guardrail.Result{}, nil
	}

	modified := text
	var triggered bool
	var firstCategory string

	for category, re := range patterns {
		if !d.categories[category] {
			continue
		}
		if !re.MatchString(text) {
			continue
		}
		triggered = true
		if firstCategory == "" {
			firstCategory = category
		}
		if d.action == guardrail.ActionMask {
			cat := category
			modified = re.ReplaceAllStringFunc(modified, func(_ string) string {
				return fmt.Sprintf("[%s_REDACTED]", strings.ToUpper(cat))
			})
		}
	}

	if !triggered {
		return &guardrail.Result{}, nil
	}

	return &guardrail.Result{
		Triggered: true,
		Action:    d.action,
		Modified:  modified,
		Category:  firstCategory,
		Guardrail: "pii",
	}, nil
}
