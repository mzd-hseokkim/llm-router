package redistore

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const budgetKeyTTL = 25 * time.Hour // slightly longer than a day to survive daily resets

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

// IncrSpend atomically adds amountUSD and returns the new total spend.
func (c *BudgetCache) IncrSpend(ctx context.Context, entityType string, entityID uuid.UUID, period string, amountUSD float64) (float64, error) {
	key := budgetKey(entityType, entityID, period)
	result, err := c.client.IncrByFloat(ctx, key, amountUSD).Result()
	if err != nil {
		return 0, fmt.Errorf("budget cache incr: %w", err)
	}
	// Set TTL on first write (INCRBYFLOAT preserves TTL if key already has one,
	// but not on creation).
	c.client.Expire(ctx, key, budgetKeyTTL)
	return result, nil
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
	val := strconv.FormatFloat(amountUSD, 'f', -1, 64)
	if err := c.client.Set(ctx, key, val, budgetKeyTTL).Err(); err != nil {
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
