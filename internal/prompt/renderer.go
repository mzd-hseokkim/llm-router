// Package prompt implements prompt template management: storage, versioning,
// variable rendering, and injection into chat requests.
package prompt

import (
	"fmt"
	"regexp"
	"strings"
)

var varPattern = regexp.MustCompile(`\{\{(\w+)\}\}`)

// Render replaces all {{variable}} placeholders in template with the provided
// values. Returns an error if a required variable is missing.
func Render(template string, variables []VariableDef, values map[string]string) (string, error) {
	// Build a lookup map of required/default variables.
	defs := make(map[string]VariableDef, len(variables))
	for _, v := range variables {
		defs[v.Name] = v
	}

	var missing []string
	result := varPattern.ReplaceAllStringFunc(template, func(match string) string {
		// Extract the name between {{ and }}.
		name := match[2 : len(match)-2]
		if val, ok := values[name]; ok {
			return val
		}
		if def, ok := defs[name]; ok {
			if def.Default != "" {
				return def.Default
			}
			if def.Required {
				missing = append(missing, name)
			}
		}
		return match // leave unknown variables untouched
	})

	if len(missing) > 0 {
		return "", fmt.Errorf("missing required variables: %s", strings.Join(missing, ", "))
	}
	return result, nil
}

// CountTokens returns a rough word-based token estimate (1 token ≈ 0.75 words).
// This avoids importing a full tokeniser for a non-critical estimate.
func CountTokens(text string) int {
	words := len(strings.Fields(text))
	tokens := int(float64(words) / 0.75)
	if tokens < 1 && len(text) > 0 {
		return 1
	}
	return tokens
}

// VariableDef describes one template variable.
type VariableDef struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`     // string | enum | number | boolean
	Required bool     `json:"required"`
	Default  string   `json:"default,omitempty"`
	Values   []string `json:"values,omitempty"` // enum values
}
