package fallback_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/llm-router/gateway/internal/gateway/circuitbreaker"
	"github.com/llm-router/gateway/internal/gateway/fallback"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

// mockProvider is a test double for provider.Provider.
type mockProvider struct {
	name    string
	resp    *types.ChatCompletionResponse
	err     error
	callCnt int
}

func (m *mockProvider) Name() string              { return m.name }
func (m *mockProvider) Models() []types.ModelInfo { return nil }
func (m *mockProvider) ChatCompletion(_ context.Context, _ string, _ *types.ChatCompletionRequest, _ []byte) (*types.ChatCompletionResponse, error) {
	m.callCnt++
	return m.resp, m.err
}
func (m *mockProvider) ChatCompletionStream(_ context.Context, _ string, _ *types.ChatCompletionRequest, _ []byte) (<-chan provider.StreamChunk, error) {
	m.callCnt++
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan provider.StreamChunk)
	close(ch)
	return ch, nil
}

func newRegistry(providers ...provider.Provider) *provider.Registry {
	r := provider.NewRegistry()
	for _, p := range providers {
		r.Register(p)
	}
	return r
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func mockResp(model string) *types.ChatCompletionResponse {
	return &types.ChatCompletionResponse{
		ID:    "test",
		Model: model,
		Choices: []types.Choice{{
			Message: types.Message{Role: "assistant", Content: "hi"},
		}},
	}
}

func retryableErr() *provider.GatewayError {
	return &provider.GatewayError{Code: provider.ErrProviderError, HTTPStatus: 500, Message: "server error"}
}

func nonRetryableErr() *provider.GatewayError {
	return &provider.GatewayError{Code: provider.ErrInvalidRequest, HTTPStatus: 400, Message: "bad request"}
}

// --- Tests ---

func TestRouter_SingleTarget_Success(t *testing.T) {
	p := &mockProvider{name: "openai", resp: mockResp("openai/gpt-4o")}
	cb := circuitbreaker.New(circuitbreaker.DefaultConfig())
	fr := fallback.NewRouter(newRegistry(p), cb, discardLogger())

	chain := fallback.SingleTarget("openai", "gpt-4o")
	resp, used, err := fr.Execute(context.Background(), chain, &types.ChatCompletionRequest{}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if used.Provider != "openai" {
		t.Errorf("used.Provider = %q, want openai", used.Provider)
	}
}

func TestRouter_Fallback_OnRetryableError(t *testing.T) {
	primary := &mockProvider{name: "openai", err: retryableErr()}
	secondary := &mockProvider{name: "anthropic", resp: mockResp("anthropic/claude")}

	cb := circuitbreaker.New(circuitbreaker.Config{
		FailureThreshold: 10, // high threshold so CB doesn't open during test
		SuccessThreshold: 2,
	})
	fr := fallback.NewRouter(newRegistry(primary, secondary), cb, discardLogger())

	chain := fallback.Chain{
		Name: "test",
		Targets: []fallback.Target{
			{Provider: "openai", Model: "gpt-4o"},
			{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
		},
	}

	resp, used, err := fr.Execute(context.Background(), chain, &types.ChatCompletionRequest{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used.Provider != "anthropic" {
		t.Errorf("used.Provider = %q, want anthropic (should have fallen back)", used.Provider)
	}
	if resp == nil {
		t.Fatal("expected response from fallback")
	}
}

func TestRouter_NoFallback_OnNonRetryableError(t *testing.T) {
	primary := &mockProvider{name: "openai", err: nonRetryableErr()}
	secondary := &mockProvider{name: "anthropic", resp: mockResp("anthropic/claude")}

	cb := circuitbreaker.New(circuitbreaker.DefaultConfig())
	fr := fallback.NewRouter(newRegistry(primary, secondary), cb, discardLogger())

	chain := fallback.Chain{
		Name: "test",
		Targets: []fallback.Target{
			{Provider: "openai", Model: "gpt-4o"},
			{Provider: "anthropic", Model: "claude"},
		},
	}

	_, _, err := fr.Execute(context.Background(), chain, &types.ChatCompletionRequest{}, nil)
	if err == nil {
		t.Fatal("expected error from non-retryable failure")
	}
	// Secondary should NOT have been called.
	if secondary.callCnt > 0 {
		t.Errorf("secondary provider should not be called on non-retryable error; callCnt = %d", secondary.callCnt)
	}
}

func TestRouter_AllFallbacksExhausted(t *testing.T) {
	p1 := &mockProvider{name: "openai", err: retryableErr()}
	p2 := &mockProvider{name: "anthropic", err: retryableErr()}

	cb := circuitbreaker.New(circuitbreaker.Config{
		FailureThreshold: 10,
		SuccessThreshold: 2,
	})
	fr := fallback.NewRouter(newRegistry(p1, p2), cb, discardLogger())

	chain := fallback.Chain{
		Name: "test",
		Targets: []fallback.Target{
			{Provider: "openai", Model: "gpt-4o"},
			{Provider: "anthropic", Model: "claude"},
		},
	}

	_, _, err := fr.Execute(context.Background(), chain, &types.ChatCompletionRequest{}, nil)
	if err == nil {
		t.Fatal("expected error when all fallbacks exhausted")
	}
	if !errors.Is(err, retryableErr()) {
		// The error is wrapped; just check it's non-nil with the right structure.
		var gwErr *provider.GatewayError
		if !errors.As(err, &gwErr) {
			t.Errorf("expected GatewayError in chain, got %T: %v", err, err)
		}
	}
}

func TestRouter_SkipsOpenCircuit(t *testing.T) {
	primary := &mockProvider{name: "openai", err: retryableErr()}
	secondary := &mockProvider{name: "anthropic", resp: mockResp("anthropic/claude")}

	cb := circuitbreaker.New(circuitbreaker.Config{
		FailureThreshold: 1,            // open after 1 recorded fallback failure
		SuccessThreshold: 2,
		OpenTimeout:      time.Minute,  // stays Open during test
	})
	fr := fallback.NewRouter(newRegistry(primary, secondary), cb, discardLogger())

	chain := fallback.Chain{
		Name: "test",
		Targets: []fallback.Target{
			{Provider: "openai", Model: "gpt-4o"},
			{Provider: "anthropic", Model: "claude"},
		},
	}

	// First call: openai fails (retried) → circuit opens → fallback to anthropic.
	_, _, _ = fr.Execute(context.Background(), chain, &types.ChatCompletionRequest{}, nil)

	// Second call: openai circuit is Open → must be skipped entirely.
	primary.callCnt = 0
	secondary.callCnt = 0
	resp, used, err := fr.Execute(context.Background(), chain, &types.ChatCompletionRequest{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used.Provider != "anthropic" {
		t.Errorf("used.Provider = %q, want anthropic (openai circuit open)", used.Provider)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
	if primary.callCnt != 0 {
		t.Errorf("openai was called %d times despite open circuit; should be 0", primary.callCnt)
	}
}

func TestRouter_ContextCancellation_NoFallback(t *testing.T) {
	primary := &mockProvider{name: "openai", err: context.Canceled}
	secondary := &mockProvider{name: "anthropic", resp: mockResp("anthropic/claude")}

	cb := circuitbreaker.New(circuitbreaker.DefaultConfig())
	fr := fallback.NewRouter(newRegistry(primary, secondary), cb, discardLogger())

	chain := fallback.Chain{
		Name: "test",
		Targets: []fallback.Target{
			{Provider: "openai", Model: "gpt-4o"},
			{Provider: "anthropic", Model: "claude"},
		},
	}

	_, _, err := fr.Execute(context.Background(), chain, &types.ChatCompletionRequest{}, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if secondary.callCnt > 0 {
		t.Error("should not fall back on context cancellation")
	}
}

func TestRouter_Stream_Fallback(t *testing.T) {
	primary := &mockProvider{name: "openai", err: retryableErr()}
	secondary := &mockProvider{name: "anthropic", resp: mockResp("anthropic/claude")}

	cb := circuitbreaker.New(circuitbreaker.Config{FailureThreshold: 10, SuccessThreshold: 2})
	fr := fallback.NewRouter(newRegistry(primary, secondary), cb, discardLogger())

	chain := fallback.Chain{
		Name: "test",
		Targets: []fallback.Target{
			{Provider: "openai", Model: "gpt-4o"},
			{Provider: "anthropic", Model: "claude"},
		},
	}

	ch, used, err := fr.ExecuteStream(context.Background(), chain, &types.ChatCompletionRequest{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used.Provider != "anthropic" {
		t.Errorf("used.Provider = %q, want anthropic", used.Provider)
	}
	if ch == nil {
		t.Fatal("expected channel, got nil")
	}
}
