package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	cacheTTL    = time.Minute
	cachePrefix = "vk:auth:"
)

// Cache defines the interface for key validation result caching.
type Cache interface {
	Get(ctx context.Context, keyHash string) (*VirtualKey, error)
	Set(ctx context.Context, keyHash string, key *VirtualKey) error
	Delete(ctx context.Context, keyHash string) error
}

// RedisCache implements Cache using Redis.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis-backed cache.
func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

func (c *RedisCache) cacheKey(keyHash string) string {
	return cachePrefix + keyHash
}

// Get retrieves a cached VirtualKey by its hash. Returns (nil, nil) on cache miss.
func (c *RedisCache) Get(ctx context.Context, keyHash string) (*VirtualKey, error) {
	data, err := c.client.Get(ctx, c.cacheKey(keyHash)).Bytes()
	if err == redis.Nil {
		return nil, nil // cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var key VirtualKey
	if err := json.Unmarshal(data, &key); err != nil {
		return nil, fmt.Errorf("unmarshal cached key: %w", err)
	}
	return &key, nil
}

// Set stores a VirtualKey in cache with a 1-minute TTL.
func (c *RedisCache) Set(ctx context.Context, keyHash string, key *VirtualKey) error {
	data, err := json.Marshal(key)
	if err != nil {
		return fmt.Errorf("marshal key for cache: %w", err)
	}
	if err := c.client.Set(ctx, c.cacheKey(keyHash), data, cacheTTL).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

// Delete removes a cached key, e.g. after deactivation.
func (c *RedisCache) Delete(ctx context.Context, keyHash string) error {
	if err := c.client.Del(ctx, c.cacheKey(keyHash)).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	return nil
}
