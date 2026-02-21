package retry

import (
	"context"
	"time"
)

// Execute calls fn up to policy.MaxAttempts times, waiting between retries.
// It stops early if:
//   - fn returns nil (success)
//   - fn returns a non-retryable error
//   - ctx is cancelled between retries
func Execute(ctx context.Context, policy RetryPolicy, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		if attempt > 0 {
			delay := policy.Delay(attempt-1, lastErr)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !policy.ShouldRetry(lastErr) {
			return lastErr
		}
	}
	return lastErr
}
