package residency_test

import (
	"testing"

	"github.com/llm-router/gateway/internal/gateway/fallback"
	"github.com/llm-router/gateway/internal/residency"
)

func buildRegistry() *residency.Registry {
	euPolicy := &residency.Policy{
		Name: "eu-only",
		AllowedProviders: map[string]residency.ProviderConstraint{
			"azure":   {Name: "azure", Region: "westeurope"},
			"bedrock": {Name: "bedrock", Region: "eu-west-1"},
		},
		BlockedProviders: map[string]bool{},
	}
	hipaaPolicy := &residency.Policy{
		Name:             "hipaa",
		AllowedProviders: map[string]residency.ProviderConstraint{},
		BlockedProviders: map[string]bool{
			"openai":    true,
			"anthropic": true,
			"gemini":    true,
		},
	}
	return residency.NewRegistry([]*residency.Policy{euPolicy, hipaaPolicy})
}

func chain(providers ...string) fallback.Chain {
	targets := make([]fallback.Target, len(providers))
	for i, p := range providers {
		targets[i] = fallback.Target{Provider: p, Model: "some-model"}
	}
	return fallback.Chain{Name: "test", Targets: targets}
}

func TestFilterChain_AllowedProvider(t *testing.T) {
	e := residency.NewEnforcer(buildRegistry())
	got, err := e.FilterChain("eu-only", chain("azure", "openai", "bedrock"), "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(got.Targets))
	}
	if got.Targets[0].Provider != "azure" || got.Targets[1].Provider != "bedrock" {
		t.Errorf("unexpected targets: %v", got.Targets)
	}
}

func TestFilterChain_NoCompliant(t *testing.T) {
	e := residency.NewEnforcer(buildRegistry())
	_, err := e.FilterChain("eu-only", chain("openai", "anthropic", "gemini"), "gpt-4o")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFilterChain_UnknownPolicy(t *testing.T) {
	e := residency.NewEnforcer(buildRegistry())
	got, err := e.FilterChain("unknown-policy", chain("openai"), "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Targets) != 1 {
		t.Fatalf("expected chain unchanged, got %d targets", len(got.Targets))
	}
}

func TestFilterChain_BlocklistDenylist(t *testing.T) {
	e := residency.NewEnforcer(buildRegistry())
	// hipaa policy blocks openai/anthropic/gemini, but allows all others
	got, err := e.FilterChain("hipaa", chain("openai", "azure", "bedrock"), "claude-sonnet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(got.Targets))
	}
}

func TestCheckProvider_Blocked(t *testing.T) {
	e := residency.NewEnforcer(buildRegistry())
	err := e.CheckProvider("eu-only", "gemini", "gemini-2.0-flash")
	if err == nil {
		t.Fatal("expected error for non-EU provider")
	}
}

func TestCheckProvider_Allowed(t *testing.T) {
	e := residency.NewEnforcer(buildRegistry())
	err := e.CheckProvider("eu-only", "azure", "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
