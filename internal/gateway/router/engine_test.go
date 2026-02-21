package router

import (
	"context"
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
)

func TestAdvancedRouter_Resolve(t *testing.T) {
	rules := []types.RouteRule{
		{
			Name:     "premium",
			Priority: 100,
			Enabled:  true,
			Match: types.RouteMatch{
				Metadata: map[string]string{"user_tier": "premium"},
			},
			Strategy: types.StrategyDirect,
			Targets:  []types.RuleTarget{{Provider: "anthropic", Model: "claude-opus-4-20250514"}},
		},
		{
			Name:     "long-context",
			Priority: 80,
			Enabled:  true,
			Match: types.RouteMatch{
				MinContextTokens: 1000,
			},
			Strategy: types.StrategyDirect,
			Targets:  []types.RuleTarget{{Provider: "anthropic", Model: "claude-opus-4-20250514"}},
		},
		{
			Name:     "openai-prefix",
			Priority: 50,
			Enabled:  true,
			Match: types.RouteMatch{
				ModelPrefix: "openai/",
			},
			Strategy: types.StrategyDirect,
			Targets:  []types.RuleTarget{{Provider: "openai", Model: "{model}"}},
		},
		{
			Name:     "disabled",
			Priority: 200,
			Enabled:  false,
			Match:    types.RouteMatch{},
			Strategy: types.StrategyDirect,
			Targets:  []types.RuleTarget{{Provider: "disabled", Model: "nope"}},
		},
	}

	ar, err := NewAdvancedRouter(rules, nil)
	if err != nil {
		t.Fatalf("NewAdvancedRouter: %v", err)
	}

	tests := []struct {
		name          string
		req           *types.ChatCompletionRequest
		wantMatch     bool
		wantRule      string
		wantProvider  string
	}{
		{
			name: "premium metadata matches",
			req: &types.ChatCompletionRequest{
				Model:    "gpt-4o",
				Messages: []types.Message{{Role: "user", Content: "hello"}},
				Metadata: map[string]string{"user_tier": "premium"},
			},
			wantMatch:    true,
			wantRule:     "premium",
			wantProvider: "anthropic",
		},
		{
			name: "long context triggers rule",
			req: &types.ChatCompletionRequest{
				Model: "gpt-4o",
				// ~4000 tokens (4 chars each × 16000 chars)
				Messages: []types.Message{{Role: "user", Content: string(make([]byte, 4001*4))}},
			},
			wantMatch:    true,
			wantRule:     "long-context",
			wantProvider: "anthropic",
		},
		{
			name: "openai prefix match",
			req: &types.ChatCompletionRequest{
				Model:    "openai/gpt-4o",
				Messages: []types.Message{{Role: "user", Content: "hi"}},
			},
			wantMatch:    true,
			wantRule:     "openai-prefix",
			wantProvider: "openai",
		},
		{
			name: "no rule matches",
			req: &types.ChatCompletionRequest{
				Model:    "unknown-model",
				Messages: []types.Message{{Role: "user", Content: "hi"}},
			},
			wantMatch: false,
		},
		{
			name: "disabled rule not matched",
			req: &types.ChatCompletionRequest{
				Model:    "anything",
				Messages: []types.Message{{Role: "user", Content: "hi"}},
			},
			wantMatch: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chain, matched := ar.Resolve(context.Background(), tc.req)
			if matched != tc.wantMatch {
				t.Fatalf("Resolve matched=%v want=%v", matched, tc.wantMatch)
			}
			if !tc.wantMatch {
				return
			}
			if chain.Name != tc.wantRule {
				t.Errorf("chain.Name=%q want=%q", chain.Name, tc.wantRule)
			}
			if len(chain.Targets) == 0 {
				t.Fatal("no targets in chain")
			}
			if chain.Targets[0].Provider != tc.wantProvider {
				t.Errorf("target.Provider=%q want=%q", chain.Targets[0].Provider, tc.wantProvider)
			}
		})
	}
}

func TestAdvancedRouter_Reload(t *testing.T) {
	ar, _ := NewAdvancedRouter(nil, nil)

	rules := []types.RouteRule{{
		Name:     "r1",
		Priority: 10,
		Enabled:  true,
		Match:    types.RouteMatch{Model: "gpt-4o"},
		Strategy: types.StrategyDirect,
		Targets:  []types.RuleTarget{{Provider: "openai", Model: "gpt-4o"}},
	}}

	if err := ar.Reload(rules); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	chain, matched := ar.Resolve(context.Background(), &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "x"}},
	})
	if !matched {
		t.Fatal("expected match after reload")
	}
	if chain.Name != "r1" {
		t.Errorf("chain.Name=%q want=r1", chain.Name)
	}
}
