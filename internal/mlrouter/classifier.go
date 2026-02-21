// Package mlrouter implements ML-based intelligent routing for LLM requests.
// It uses rule-based heuristics (not an external ML runtime) to classify
// request complexity and select the most cost-effective provider.
package mlrouter

import (
	"strings"
	"unicode"

	"github.com/llm-router/gateway/internal/gateway/types"
)

// Complexity levels for a request.
const (
	ComplexitySimple  = "economy"  // short, simple tasks
	ComplexityMedium  = "medium"   // coding, analysis, writing
	ComplexityComplex = "premium"  // complex reasoning, research
)

// Features holds the extracted features for a single request.
type Features struct {
	TokenEstimate    int     // estimated number of input tokens
	MessageCount     int     // number of turns in the conversation
	HasCodeBlocks    bool    // at least one ``` code block present
	HasToolCalls     bool    // tools array is non-empty
	TechnicalDensity float64 // fraction of "technical" words in the prompt
	ComplexityScore  float64 // 0.0 (simple) – 1.0 (complex)
	Tier             string  // economy | medium | premium
}

// Classifier extracts features from a chat completion request and assigns a
// complexity tier using rule-based heuristics.
type Classifier struct{}

// NewClassifier creates a Classifier.
func NewClassifier() *Classifier { return &Classifier{} }

// Classify returns the extracted features and the recommended quality tier.
func (c *Classifier) Classify(req *types.ChatCompletionRequest) Features {
	f := Features{
		MessageCount: len(req.Messages),
		HasToolCalls: len(req.Tools) > 0,
	}

	// Concatenate all message content for analysis.
	var sb strings.Builder
	for _, m := range req.Messages {
		sb.WriteString(m.Content)
		sb.WriteByte(' ')
	}
	text := sb.String()

	f.TokenEstimate = estimateTokens(text)
	f.HasCodeBlocks = strings.Contains(text, "```")
	f.TechnicalDensity = technicalDensity(text)

	// Composite complexity score [0, 1]:
	//
	//   token length (capped at 100 tokens):  weight 0.40
	//   code blocks present:                   +0.20 flat
	//   multi-turn depth (capped at 6):        weight 0.15
	//   technical term density:                weight 0.15
	//   tool calls present:                    +0.10 flat
	//
	// Tier thresholds: economy < 0.25, medium 0.25–0.45, premium ≥ 0.45
	score := 0.0
	score += clamp(float64(f.TokenEstimate)/100.0) * 0.40
	if f.HasCodeBlocks {
		score += 0.20
	}
	score += clamp(float64(f.MessageCount)/6.0) * 0.15
	score += f.TechnicalDensity * 0.15
	if f.HasToolCalls {
		score += 0.10
	}

	f.ComplexityScore = clamp(score)

	switch {
	case f.ComplexityScore >= 0.45:
		f.Tier = ComplexityComplex
	case f.ComplexityScore >= 0.25:
		f.Tier = ComplexityMedium
	default:
		f.Tier = ComplexitySimple
	}

	return f
}

// estimateTokens approximates the token count of text using the common
// heuristic of ~4 characters per token.
func estimateTokens(text string) int {
	return len(text) / 4
}

// technicalDensity returns the fraction of words that appear to be technical
// (contain digits, underscores, camelCase, or are known technical keywords).
func technicalDensity(text string) float64 {
	words := strings.Fields(text)
	if len(words) == 0 {
		return 0
	}
	var tech int
	for _, w := range words {
		if isTechnical(w) {
			tech++
		}
	}
	return clamp(float64(tech) / float64(len(words)) * 3) // amplify slightly
}

var technicalKeywords = map[string]bool{
	"function": true, "class": true, "interface": true, "algorithm": true,
	"optimize": true, "complexity": true, "runtime": true, "async": true,
	"goroutine": true, "concurrency": true, "distributed": true, "kubernetes": true,
	"tensor": true, "gradient": true, "neural": true, "regression": true,
	"theorem": true, "proof": true, "calculus": true, "differential": true,
	"api": true, "sql": true, "http": true, "json": true, "yaml": true,
}

func isTechnical(w string) bool {
	w = strings.ToLower(strings.Trim(w, ".,;:!?\"'()[]{}"))
	if technicalKeywords[w] {
		return true
	}
	// Contains digit (version numbers, hex, etc.)
	for _, r := range w {
		if unicode.IsDigit(r) {
			return true
		}
	}
	// Contains underscore or camelCase
	if strings.Contains(w, "_") {
		return true
	}
	for i, r := range w {
		if i > 0 && unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
