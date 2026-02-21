package injection

import (
	"context"
	"regexp"

	"github.com/llm-router/gateway/internal/guardrail"
)

var injectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore\s+.{0,30}instructions?`),
	regexp.MustCompile(`(?i)you\s+are\s+now\s+`),
	regexp.MustCompile(`(?i)act\s+as\s+(if\s+you\s+are|a|an)\s+`),
	regexp.MustCompile(`(?i)forget\s+(everything|all|your)\s+(you|previous)`),
	regexp.MustCompile(`(?i)(system\s+prompt|initial\s+instructions)`),
	regexp.MustCompile(`(?i)(jailbreak|dan\s+mode|developer\s+mode)`),
}

// Detector detects prompt injection attempts in input text.
type Detector struct {
	enabled bool
	action  guardrail.Action
}

// NewDetector creates a prompt injection detector.
func NewDetector(enabled bool, action guardrail.Action) *Detector {
	return &Detector{enabled: enabled, action: action}
}

func (d *Detector) Name() string { return "prompt_injection" }

func (d *Detector) Check(_ context.Context, text string, dir guardrail.Direction) (*guardrail.Result, error) {
	if !d.enabled || dir != guardrail.DirectionInput {
		return &guardrail.Result{}, nil
	}

	for _, re := range injectionPatterns {
		if re.MatchString(text) {
			return &guardrail.Result{
				Triggered: true,
				Action:    d.action,
				Category:  "prompt_injection",
				Guardrail: "prompt_injection",
			}, nil
		}
	}
	return &guardrail.Result{}, nil
}
