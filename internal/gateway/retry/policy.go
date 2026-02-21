package retry

import (
	"errors"
	"math/rand"
	"time"

	"github.com/llm-router/gateway/internal/provider"
)

// RetryPolicy defines exponential backoff with jitter parameters.
type RetryPolicy struct {
	MaxAttempts  int           // total attempts including the first; default 3
	InitialDelay time.Duration // delay after first failure; default 500ms
	MaxDelay     time.Duration // cap on computed delay; default 30s
	Multiplier   float64       // backoff multiplier per attempt; default 2.0
	JitterFactor float64       // ±jitter fraction applied to delay; default 0.25
}

// Default returns a RetryPolicy with production-suitable defaults.
func Default() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:  3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		JitterFactor: 0.25,
	}
}

// ShouldRetry reports whether the given error warrants a retry.
func (p RetryPolicy) ShouldRetry(err error) bool {
	if err == nil {
		return false
	}
	var gwErr *provider.GatewayError
	if errors.As(err, &gwErr) {
		return gwErr.IsRetryable()
	}
	// Non-GatewayErrors (e.g. context cancellation) are not retried.
	return false
}

// Delay returns the wait duration before the given attempt (0-based after the first failure).
// If the error carries a Retry-After hint it is used directly (capped at MaxDelay).
func (p RetryPolicy) Delay(attempt int, err error) time.Duration {
	if err != nil {
		var gwErr *provider.GatewayError
		if errors.As(err, &gwErr) && gwErr.RetryAfter > 0 {
			if gwErr.RetryAfter > p.MaxDelay {
				return p.MaxDelay
			}
			return gwErr.RetryAfter
		}
	}
	return p.calcBackoff(attempt)
}

// calcBackoff computes exponential backoff for attempt (0-based), with jitter.
func (p RetryPolicy) calcBackoff(attempt int) time.Duration {
	delay := float64(p.InitialDelay)
	for i := 0; i < attempt; i++ {
		delay *= p.Multiplier
	}
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}
	// Apply symmetric ±JitterFactor.
	jitter := delay * p.JitterFactor * (2*rand.Float64() - 1)
	result := time.Duration(delay + jitter)
	if result < 0 {
		result = 0
	}
	return result
}
