package ratelimit

import (
	"context"
	"errors"
	"time"
)

// ErrRateLimitExceeded is returned when a rate limit check fails.
var ErrRateLimitExceeded = errors.New("rate limit exceeded")

// Metric identifies what is being counted.
type Metric string

const (
	MetricRPM Metric = "rpm" // requests per minute
	MetricTPM Metric = "tpm" // tokens per minute
)

// Result is the outcome of a rate limit check.
type Result struct {
	Allowed   bool
	Remaining int
	ResetAt   time.Time // when the current window resets (zero if not known)
}

// Limiter checks and increments a sliding-window counter stored in Redis.
type Limiter interface {
	// Allow checks whether cost units can be consumed for the given key.
	// If allowed, the counter is atomically incremented.
	Allow(ctx context.Context, key string, limit, cost int, window time.Duration) (Result, error)
}
