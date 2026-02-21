package alerting

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Deduplicator prevents the same alert from being sent repeatedly within a time window.
type Deduplicator struct {
	redis  *redis.Client
	window time.Duration
}

// NewDeduplicator creates a Deduplicator with the given suppression window.
func NewDeduplicator(r *redis.Client, window time.Duration) *Deduplicator {
	if window == 0 {
		window = 15 * time.Minute
	}
	return &Deduplicator{redis: r, window: window}
}

// ShouldSend returns true if the alert should be sent (not a duplicate).
// It atomically marks the key as sent so concurrent callers agree.
func (d *Deduplicator) ShouldSend(ctx context.Context, eventType, entityID string) bool {
	key := fmt.Sprintf("alert:dedup:%s:%s", eventType, entityID)
	ok, err := d.redis.SetNX(ctx, key, 1, d.window).Result()
	if err != nil {
		// On Redis error, allow the alert to pass through.
		return true
	}
	return ok
}
