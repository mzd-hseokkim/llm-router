// Package residency enforces data residency policies by filtering provider
// fallback chains so that only compliant providers are used.
package residency

import "context"

// ctxKey is the unexported context key for the active policy name.
type ctxKey struct{}

// WithPolicy stores the residency policy name in ctx.
func WithPolicy(ctx context.Context, policyName string) context.Context {
	return context.WithValue(ctx, ctxKey{}, policyName)
}

// PolicyFromContext retrieves the residency policy name from ctx.
// Returns "" if none was set.
func PolicyFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}

// Policy defines which providers are allowed or blocked.
type Policy struct {
	Name string
	// AllowedProviders is the allowlist of provider names.
	// Empty means all providers are allowed (subject to BlockedProviders).
	AllowedProviders map[string]ProviderConstraint
	// BlockedProviders is the denylist; takes precedence over AllowedProviders.
	BlockedProviders map[string]bool
	// AllowedRegions is informational; stored for reporting.
	AllowedRegions []string
}

// ProviderConstraint holds optional region metadata for an allowed provider.
type ProviderConstraint struct {
	Name   string
	Region string
}

// IsProviderAllowed returns true if providerName passes this policy.
func (p *Policy) IsProviderAllowed(providerName string) bool {
	if p.BlockedProviders[providerName] {
		return false
	}
	if len(p.AllowedProviders) == 0 {
		return true // no allowlist = all providers permitted
	}
	_, ok := p.AllowedProviders[providerName]
	return ok
}

// AllowedProviderNames returns the names of explicitly allowed providers.
func (p *Policy) AllowedProviderNames() []string {
	names := make([]string, 0, len(p.AllowedProviders))
	for n := range p.AllowedProviders {
		names = append(names, n)
	}
	return names
}

// Registry holds a set of named policies.
type Registry struct {
	policies map[string]*Policy
}

// NewRegistry builds a Registry from a slice of policies.
func NewRegistry(policies []*Policy) *Registry {
	m := make(map[string]*Policy, len(policies))
	for _, p := range policies {
		m[p.Name] = p
	}
	return &Registry{policies: m}
}

// Get returns the policy with the given name.
func (r *Registry) Get(name string) (*Policy, bool) {
	p, ok := r.policies[name]
	return p, ok
}

// List returns all registered policies.
func (r *Registry) List() []*Policy {
	out := make([]*Policy, 0, len(r.policies))
	for _, p := range r.policies {
		out = append(out, p)
	}
	return out
}
