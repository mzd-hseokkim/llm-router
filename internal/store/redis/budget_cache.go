package redistore

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// budgetTTL returns the cache TTL for a given budget period. The TTL is set
// slightly longer than the period so the key survives a period reset and is
// reseeded from DB rather than starting from zero.
func budgetTTL(period string) time.Duration {
	switch period {
	case "hourly":
		return 2 * time.Hour
	case "daily":
		return 25 * time.Hour
	case "weekly":
		return 8 * 24 * time.Hour
	case "monthly":
		return 32 * 24 * time.Hour
	default: // lifetime and unknown periods
		return 366 * 24 * time.Hour
	}
}

// BudgetCache implements budget.Cache using Redis INCRBYFLOAT.
type BudgetCache struct {
	client *redis.Client
}

// NewBudgetCache creates a BudgetCache backed by client.
func NewBudgetCache(client *redis.Client) *BudgetCache {
	return &BudgetCache{client: client}
}

func budgetKey(entityType string, entityID uuid.UUID, period string) string {
	return fmt.Sprintf("budget:%s:%s:%s", entityType, entityID, period)
}

// IncrSpend atomically increments the spend counter and sets the TTL only on
// first creation (ExpireNX). Both commands are pipelined to minimise round trips.
func (c *BudgetCache) IncrSpend(ctx context.Context, entityType string, entityID uuid.UUID, period string, amountUSD float64) (float64, error) {
	key := budgetKey(entityType, entityID, period)
	ttl := budgetTTL(period)

	pipe := c.client.Pipeline()
	incrCmd := pipe.IncrByFloat(ctx, key, amountUSD)
	pipe.ExpireNX(ctx, key, ttl) // only set TTL when key is first created
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("budget cache incr: %w", err)
	}
	return incrCmd.Val(), nil
}

// GetSpend returns the current spend. ok=false means the key does not exist.
func (c *BudgetCache) GetSpend(ctx context.Context, entityType string, entityID uuid.UUID, period string) (float64, bool, error) {
	key := budgetKey(entityType, entityID, period)
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("budget cache get: %w", err)
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, false, fmt.Errorf("budget cache parse: %w", err)
	}
	return f, true, nil
}

// SetSpend seeds the cache with a value from DB (used on cache miss).
func (c *BudgetCache) SetSpend(ctx context.Context, entityType string, entityID uuid.UUID, period string, amountUSD float64) error {
	key := budgetKey(entityType, entityID, period)
	ttl := budgetTTL(period)
	val := strconv.FormatFloat(amountUSD, 'f', -1, 64)
	if err := c.client.Set(ctx, key, val, ttl).Err(); err != nil {
		return fmt.Errorf("budget cache set: %w", err)
	}
	return nil
}

// DeleteSpend removes the cache key (called on period reset).
func (c *BudgetCache) DeleteSpend(ctx context.Context, entityType string, entityID uuid.UUID, period string) error {
	key := budgetKey(entityType, entityID, period)
	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("budget cache delete: %w", err)
	}
	return nil
}
