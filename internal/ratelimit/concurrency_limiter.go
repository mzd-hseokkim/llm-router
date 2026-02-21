package ratelimit

import (
	"context"
	"errors"
)

// ErrTooManyConcurrentRequests is returned when the concurrency cap is reached.
var ErrTooManyConcurrentRequests = errors.New("too many concurrent requests")

// ConcurrencyLimiter limits the number of simultaneously in-flight requests
// using a buffered channel as a semaphore.
type ConcurrencyLimiter struct {
	sem chan struct{}
}

// NewConcurrencyLimiter creates a limiter that allows at most max concurrent requests.
func NewConcurrencyLimiter(max int) *ConcurrencyLimiter {
	return &ConcurrencyLimiter{sem: make(chan struct{}, max)}
}

// Acquire tries to reserve a concurrency slot.
// Returns a release function on success. Returns ErrTooManyConcurrentRequests
// immediately (non-blocking) if the limit is already reached.
// If ctx is cancelled while waiting, ctx.Err() is returned.
func (l *ConcurrencyLimiter) Acquire(ctx context.Context) (release func(), err error) {
	select {
	case l.sem <- struct{}{}:
		return func() { <-l.sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, ErrTooManyConcurrentRequests
	}
}
