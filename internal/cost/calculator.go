package cost

import (
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ModelPricing holds per-token pricing for a model.
type ModelPricing struct {
	Provider               string  `yaml:"provider"`
	InputPerMillionTokens  float64 `yaml:"input_per_million_tokens"`
	OutputPerMillionTokens float64 `yaml:"output_per_million_tokens"`
}

// modelsYAML is the on-disk structure for config/models.yaml.
type modelsYAML struct {
	Models map[string]ModelPricing `yaml:"models"`
}

// Calculator computes LLM request costs from token counts.
type Calculator struct {
	models map[string]ModelPricing
}

// NewCalculator creates a Calculator with the given pricing table.
func NewCalculator(models map[string]ModelPricing) *Calculator {
	return &Calculator{models: models}
}

// LoadFromYAML reads model pricing from a YAML file.
func LoadFromYAML(path string) (*Calculator, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read models config: %w", err)
	}
	var cfg modelsYAML
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse models config: %w", err)
	}
	if cfg.Models == nil {
		cfg.Models = map[string]ModelPricing{}
	}
	return NewCalculator(cfg.Models), nil
}

// Calculate returns the USD cost for a request.
// Model lookup is case-insensitive and strips provider prefixes like "openai/".
// Returns 0 if the model is not in the pricing table.
func (c *Calculator) Calculate(model string, promptTokens, completionTokens int) float64 {
	p, ok := c.lookup(model)
	if !ok {
		return 0
	}
	in := float64(promptTokens) / 1_000_000 * p.InputPerMillionTokens
	out := float64(completionTokens) / 1_000_000 * p.OutputPerMillionTokens
	return in + out
}

// UpdatePricing merges the given pricing entries into the calculator's table.
// Existing entries are overwritten; DB entries take precedence over YAML defaults.
func (c *Calculator) UpdatePricing(pricing map[string]ModelPricing) {
	for k, v := range pricing {
		c.models[strings.ToLower(k)] = v
	}
}

// HasPricing reports whether the model has a known price.
func (c *Calculator) HasPricing(model string) bool {
	_, ok := c.lookup(model)
	return ok
}

// lookup tries exact match, then strips provider prefix (e.g. "openai/gpt-4o" → "gpt-4o").
func (c *Calculator) lookup(model string) (ModelPricing, bool) {
	model = strings.ToLower(model)
	if p, ok := c.models[model]; ok {
		return p, true
	}
	// Strip "provider/" prefix
	if idx := strings.Index(model, "/"); idx >= 0 {
		if p, ok := c.models[model[idx+1:]]; ok {
			return p, true
		}
	}
	return ModelPricing{}, false
}
