package router

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"

	"github.com/llm-router/gateway/internal/cost"
	"github.com/llm-router/gateway/internal/gateway/fallback"
	"github.com/llm-router/gateway/internal/gateway/types"
)

// AdvancedRouter evaluates priority-ordered routing rules and returns a
// fallback.Chain for the first matching rule.
type AdvancedRouter struct {
	mu      sync.RWMutex
	rules   []compiledRule // sorted by priority desc
	pricing *cost.Calculator
}

// NewAdvancedRouter returns an AdvancedRouter loaded with the given rules.
func NewAdvancedRouter(rules []types.RouteRule, pricing *cost.Calculator) (*AdvancedRouter, error) {
	ar := &AdvancedRouter{pricing: pricing}
	return ar, ar.reload(rules)
}

// Reload replaces the rule set atomically. Safe for concurrent use.
func (ar *AdvancedRouter) Reload(rules []types.RouteRule) error {
	return ar.reload(rules)
}

func (ar *AdvancedRouter) reload(rules []types.RouteRule) error {
	compiled := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		cr, err := compileRule(r)
		if err != nil {
			return fmt.Errorf("compile rule %q: %w", r.Name, err)
		}
		compiled = append(compiled, cr)
	}
	// higher priority number = evaluated first
	sort.Slice(compiled, func(i, j int) bool {
		return compiled[i].Priority > compiled[j].Priority
	})

	ar.mu.Lock()
	ar.rules = compiled
	ar.mu.Unlock()
	return nil
}

// Resolve evaluates rules against the request and returns the matching chain.
// Returns (chain, true) on a match, or (zero, false) if no rule matches.
func (ar *AdvancedRouter) Resolve(ctx context.Context, req *types.ChatCompletionRequest) (fallback.Chain, bool) {
	estimated := estimateTokens(req.Messages)

	ar.mu.RLock()
	rules := ar.rules
	ar.mu.RUnlock()

	for i := range rules {
		rule := &rules[i]
		if !rule.Matches(ctx, req, estimated) {
			continue
		}
		chain, err := ar.buildChain(ctx, req, rule)
		if err != nil || len(chain.Targets) == 0 {
			continue
		}
		return chain, true
	}
	return fallback.Chain{}, false
}

// buildChain converts a matched rule into a fallback.Chain using the rule's strategy.
func (ar *AdvancedRouter) buildChain(_ context.Context, req *types.ChatCompletionRequest, rule *compiledRule) (fallback.Chain, error) {
	if len(rule.Targets) == 0 {
		return fallback.Chain{}, fmt.Errorf("rule %q has no targets", rule.Name)
	}

	targets := expandTargets(rule.Targets, req.Model)
	chain := fallback.Chain{Name: rule.Name}

	switch rule.Strategy {
	case types.StrategyDirect:
		chain.Targets = targets[:1]

	case types.StrategyWeighted:
		selected := weightedSelect(targets)
		rest := make([]fallback.Target, 0, len(targets)-1)
		for _, t := range targets {
			if t.Provider != selected.Provider || t.Model != selected.Model {
				rest = append(rest, t)
			}
		}
		chain.Targets = append([]fallback.Target{selected}, rest...)

	case types.StrategyLeastCost:
		chain.Targets = ar.sortByCost(targets, req)

	case types.StrategyQuality, types.StrategyFailover:
		chain.Targets = targets

	default:
		chain.Targets = targets
	}

	return chain, nil
}

// expandTargets converts types.RuleTarget slice into fallback.Target slice,
// expanding "{model}" placeholders with the actual request model.
func expandTargets(rts []types.RuleTarget, requestModel string) []fallback.Target {
	out := make([]fallback.Target, 0, len(rts))
	for _, rt := range rts {
		model := rt.Model
		if model == "{model}" {
			model = requestModel
		}
		out = append(out, fallback.Target{
			Provider: rt.Provider,
			Model:    model,
			Weight:   rt.Weight,
		})
	}
	return out
}

// weightedSelect picks one target using weighted random selection.
// If all weights are 0, returns a uniformly random target.
func weightedSelect(targets []fallback.Target) fallback.Target {
	total := 0
	for _, t := range targets {
		total += t.Weight
	}
	if total == 0 {
		return targets[rand.Intn(len(targets))]
	}
	r := rand.Intn(total)
	for _, t := range targets {
		r -= t.Weight
		if r < 0 {
			return t
		}
	}
	return targets[len(targets)-1]
}

// sortByCost returns targets ordered from cheapest to most expensive.
func (ar *AdvancedRouter) sortByCost(targets []fallback.Target, req *types.ChatCompletionRequest) []fallback.Target {
	if ar.pricing == nil {
		return targets
	}
	estimated := estimateTokens(req.Messages)
	type ct struct {
		t   fallback.Target
		usd float64
	}
	scored := make([]ct, len(targets))
	for i, t := range targets {
		scored[i] = ct{t, ar.pricing.Calculate(t.Model, estimated, estimated/2)}
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].usd < scored[j].usd })
	out := make([]fallback.Target, len(scored))
	for i, x := range scored {
		out[i] = x.t
	}
	return out
}

// Rules returns a read-only snapshot of the current rule set (for admin use).
func (ar *AdvancedRouter) Rules() []types.RouteRule {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	out := make([]types.RouteRule, len(ar.rules))
	for i, cr := range ar.rules {
		out[i] = cr.RouteRule
	}
	return out
}
