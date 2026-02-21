package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/llm-router/gateway/internal/gateway/retry"
	"github.com/llm-router/gateway/internal/provider"
)

func TestShouldRetry(t *testing.T) {
	p := retry.Default()
	cases := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil", nil, false},
		{"generic error", errors.New("oops"), false},
		{"rate_limit", &provider.GatewayError{Code: provider.ErrRateLimited}, true},
		{"provider_error", &provider.GatewayError{Code: provider.ErrProviderError}, true},
		{"network_error", &provider.GatewayError{Code: provider.ErrNetworkError}, true},
		{"overloaded", &provider.GatewayError{Code: provider.ErrOverloaded}, true},
		{"timeout", &provider.GatewayError{Code: provider.ErrTimeout}, true},
		{"auth_error", &provider.GatewayError{Code: provider.ErrAuthFailed}, false},
		{"invalid_request", &provider.GatewayError{Code: provider.ErrInvalidRequest}, false},
		{"model_not_found", &provider.GatewayError{Code: provider.ErrModelNotFound}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.ShouldRetry(tc.err); got != tc.expected {
				t.Errorf("ShouldRetry = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestDelay_Increases(t *testing.T) {
	p := retry.RetryPolicy{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		JitterFactor: 0, // disable jitter for deterministic test
	}
	d0 := p.Delay(0, nil)
	d1 := p.Delay(1, nil)
	d2 := p.Delay(2, nil)
	if d0 >= d1 || d1 >= d2 {
		t.Errorf("delay should increase: %v %v %v", d0, d1, d2)
	}
}

func TestDelay_MaxCap(t *testing.T) {
	p := retry.RetryPolicy{
		MaxAttempts:  10,
		InitialDelay: 1 * time.Second,
		MaxDelay:     5 * time.Second,
		Multiplier:   10.0,
		JitterFactor: 0,
	}
	if d := p.Delay(5, nil); d > p.MaxDelay {
		t.Errorf("delay %v exceeds MaxDelay %v", d, p.MaxDelay)
	}
}

func TestDelay_RetryAfterHonored(t *testing.T) {
	p := retry.Default()
	err := &provider.GatewayError{Code: provider.ErrRateLimited, RetryAfter: 7 * time.Second}
	if d := p.Delay(0, err); d != 7*time.Second {
		t.Errorf("expected Retry-After 7s, got %v", d)
	}
}

func TestDelay_RetryAfterCappedAtMax(t *testing.T) {
	p := retry.RetryPolicy{MaxDelay: 5 * time.Second, JitterFactor: 0}
	err := &provider.GatewayError{Code: provider.ErrRateLimited, RetryAfter: 60 * time.Second}
	if d := p.Delay(0, err); d != 5*time.Second {
		t.Errorf("expected delay capped at MaxDelay 5s, got %v", d)
	}
}

func TestExecute_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	if err := retry.Execute(context.Background(), retry.Default(), func() error {
		calls++
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestExecute_ExhaustsRetries(t *testing.T) {
	p := fastPolicy(3)
	calls := 0
	rateLimitErr := &provider.GatewayError{Code: provider.ErrRateLimited, HTTPStatus: 429}
	err := retry.Execute(context.Background(), p, func() error {
		calls++
		return rateLimitErr
	})
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
	if !errors.Is(err, rateLimitErr) {
		t.Errorf("expected rate limit error, got %v", err)
	}
}

func TestExecute_NoRetryOnNonRetryable(t *testing.T) {
	p := fastPolicy(3)
	calls := 0
	authErr := &provider.GatewayError{Code: provider.ErrAuthFailed, HTTPStatus: 401}
	err := retry.Execute(context.Background(), p, func() error {
		calls++
		return authErr
	})
	if calls != 1 {
		t.Errorf("expected 1 call (no retry), got %d", calls)
	}
	if err != authErr {
		t.Errorf("expected auth error, got %v", err)
	}
}

func TestExecute_SuccessAfterRetry(t *testing.T) {
	p := fastPolicy(3)
	calls := 0
	rateLimitErr := &provider.GatewayError{Code: provider.ErrRateLimited, HTTPStatus: 429}
	err := retry.Execute(context.Background(), p, func() error {
		calls++
		if calls < 3 {
			return rateLimitErr
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestExecute_ContextCancelled(t *testing.T) {
	p := retry.RetryPolicy{
		MaxAttempts:  10,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   1.0,
		JitterFactor: 0,
	}
	ctx, cancel := context.WithCancel(context.Background())
	rateLimitErr := &provider.GatewayError{Code: provider.ErrRateLimited, HTTPStatus: 429}

	done := make(chan error, 1)
	go func() {
		done <- retry.Execute(ctx, p, func() error { return rateLimitErr })
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// fastPolicy returns a policy with tiny delays for fast unit tests.
func fastPolicy(maxAttempts int) retry.RetryPolicy {
	return retry.RetryPolicy{
		MaxAttempts:  maxAttempts,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   1.0,
		JitterFactor: 0,
	}
}
