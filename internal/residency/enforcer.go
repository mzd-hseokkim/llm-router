package residency

import (
	"fmt"

	"github.com/llm-router/gateway/internal/gateway/fallback"
)

// ErrNoCompliantProvider is returned when every target in the chain is blocked
// by the active data residency policy.
type ErrNoCompliantProvider struct {
	Policy   string
	Provider string
	Model    string
}

func (e *ErrNoCompliantProvider) Error() string {
	return fmt.Sprintf(
		"data residency policy '%s' does not allow provider '%s' for model '%s'",
		e.Policy, e.Provider, e.Model,
	)
}

// Enforcer applies residency policies to fallback chains.
type Enforcer struct {
	registry *Registry
}

// NewEnforcer creates an Enforcer backed by the given registry.
func NewEnforcer(registry *Registry) *Enforcer {
	return &Enforcer{registry: registry}
}

// FilterChain removes non-compliant targets from chain according to policyName.
// Returns an error if no targets remain after filtering.
// Unknown policy names are treated as "no restriction" (fail-open).
func (e *Enforcer) FilterChain(policyName string, chain fallback.Chain, model string) (fallback.Chain, error) {
	policy, ok := e.registry.Get(policyName)
	if !ok {
		return chain, nil // unknown policy = no restriction
	}

	filtered := make([]fallback.Target, 0, len(chain.Targets))
	for _, t := range chain.Targets {
		if policy.IsProviderAllowed(t.Provider) {
			filtered = append(filtered, t)
		}
	}

	if len(filtered) == 0 {
		primary := ""
		if len(chain.Targets) > 0 {
			primary = chain.Targets[0].Provider
		}
		return fallback.Chain{}, &ErrNoCompliantProvider{
			Policy:   policyName,
			Provider: primary,
			Model:    model,
		}
	}

	return fallback.Chain{Name: chain.Name, Targets: filtered}, nil
}

// CheckProvider validates a single provider against the named policy.
// Returns nil if allowed, an error if blocked.
func (e *Enforcer) CheckProvider(policyName, providerName, model string) error {
	policy, ok := e.registry.Get(policyName)
	if !ok {
		return nil
	}
	if !policy.IsProviderAllowed(providerName) {
		return &ErrNoCompliantProvider{
			Policy:   policyName,
			Provider: providerName,
			Model:    model,
		}
	}
	return nil
}
