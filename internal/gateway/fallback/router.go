// Package fallback provides a circuit-breaker-gated fallback chain executor
// for LLM provider requests.
package fallback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/llm-router/gateway/internal/gateway/circuitbreaker"
	"github.com/llm-router/gateway/internal/gateway/retry"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

// Target is a provider+model combination in a fallback chain.
type Target struct {
	Provider string
	Model    string
	Weight   int // used by load balancers; 0 means equal weight
}

// Chain is an ordered list of targets to try on failure.
type Chain struct {
	Name    string
	Targets []Target
}

// SingleTarget returns a Chain with one target (no fallback).
func SingleTarget(providerName, model string) Chain {
	return Chain{
		Name:    providerName,
		Targets: []Target{{Provider: providerName, Model: model}},
	}
}

// Router executes requests with circuit-breaker-gated failover.
// It is safe for concurrent use.
type Router struct {
	registry *provider.Registry
	cb       *circuitbreaker.CircuitBreaker
	lb       LoadBalancer // nil means use chain order as-is
	logger   *slog.Logger
}

// NewRouter returns a Router.
func NewRouter(
	registry *provider.Registry,
	cb *circuitbreaker.CircuitBreaker,
	logger *slog.Logger,
) *Router {
	return &Router{
		registry: registry,
		cb:       cb,
		logger:   logger,
	}
}

// WithLoadBalancer attaches a LoadBalancer that reorders targets before each execution.
func (r *Router) WithLoadBalancer(lb LoadBalancer) *Router {
	r.lb = lb
	return r
}

// CircuitBreaker returns the underlying circuit breaker (for admin use).
func (r *Router) CircuitBreaker() *circuitbreaker.CircuitBreaker {
	return r.cb
}

// Execute runs the request against the fallback chain.
// It tries each target in order, skipping those with open circuits.
// Returns the response, the target that succeeded, and any error.
func (r *Router) Execute(
	ctx context.Context,
	chain Chain,
	req *types.ChatCompletionRequest,
	rawBody []byte,
) (*types.ChatCompletionResponse, Target, error) {
	targets := chain.Targets
	if r.lb != nil {
		targets = r.lb.Sort(targets)
	}

	var lastErr error
	tried := 0
	for i, t := range targets {
		if r.cb.IsOpen(t.Provider) {
			r.logger.Debug("circuit open, skipping provider",
				"provider", t.Provider, "chain", chain.Name, "attempt", i+1)
			continue
		}

		p, ok := r.registry.Get(t.Provider)
		if !ok {
			r.logger.Debug("provider not registered, skipping",
				"provider", t.Provider)
			continue
		}

		tried++
		var resp *types.ChatCompletionResponse
		err := retry.Execute(ctx, retry.Default(), func() error {
			var e error
			resp, e = p.ChatCompletion(ctx, t.Model, req, rawBody)
			return e
		})

		if err == nil {
			r.cb.RecordSuccess(t.Provider)
			if i > 0 {
				r.logger.Info("fallback succeeded",
					"chain", chain.Name,
					"provider", t.Provider,
					"model", t.Model,
					"original_provider", targets[0].Provider,
					"attempt", i+1)
			}
			return resp, t, nil
		}

		if isFallbackError(ctx, err) {
			r.cb.RecordFailure(t.Provider)
			lastErr = err
			r.logger.Warn("provider failed, trying next fallback",
				"chain", chain.Name,
				"provider", t.Provider,
				"model", t.Model,
				"error", err,
				"attempt", i+1)
			continue
		}

		// Non-retryable error (e.g. 400 bad request): return immediately.
		return nil, t, err
	}

	if tried == 0 {
		return nil, Target{}, fmt.Errorf("no available providers in chain %q (all circuits open)", chain.Name)
	}
	if lastErr != nil {
		return nil, Target{}, fmt.Errorf("all fallbacks exhausted for chain %q: %w", chain.Name, lastErr)
	}
	return nil, Target{}, fmt.Errorf("no providers in fallback chain %q", chain.Name)
}

// ExecuteStream opens a streaming channel, trying targets in order.
// Fallback is only possible before the stream is committed to the client.
func (r *Router) ExecuteStream(
	ctx context.Context,
	chain Chain,
	req *types.ChatCompletionRequest,
	rawBody []byte,
) (<-chan provider.StreamChunk, Target, error) {
	targets := chain.Targets
	if r.lb != nil {
		targets = r.lb.Sort(targets)
	}

	var lastErr error
	tried := 0
	for i, t := range targets {
		if r.cb.IsOpen(t.Provider) {
			continue
		}

		p, ok := r.registry.Get(t.Provider)
		if !ok {
			continue
		}

		tried++
		ch, err := p.ChatCompletionStream(ctx, t.Model, req, rawBody)
		if err == nil {
			r.cb.RecordSuccess(t.Provider)
			if i > 0 {
				r.logger.Info("stream fallback succeeded",
					"chain", chain.Name,
					"provider", t.Provider,
					"model", t.Model,
					"original_provider", targets[0].Provider)
			}
			return ch, t, nil
		}

		if isFallbackError(ctx, err) {
			r.cb.RecordFailure(t.Provider)
			lastErr = err
			continue
		}

		return nil, t, err
	}

	if tried == 0 {
		return nil, Target{}, fmt.Errorf("no available providers in chain %q (all circuits open)", chain.Name)
	}
	if lastErr != nil {
		return nil, Target{}, fmt.Errorf("all stream fallbacks exhausted for chain %q: %w", chain.Name, lastErr)
	}
	return nil, Target{}, fmt.Errorf("no providers in fallback chain %q", chain.Name)
}

// isFallbackError reports whether err warrants trying the next provider.
// Context cancellation/deadline is NOT retried (respects client intent).
// GatewayErrors are retried only if IsRetryable() is true.
// Unknown errors (network faults, etc.) are retried across providers.
func isFallbackError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var gwErr *provider.GatewayError
	if errors.As(err, &gwErr) {
		return gwErr.IsRetryable()
	}
	return true
}
