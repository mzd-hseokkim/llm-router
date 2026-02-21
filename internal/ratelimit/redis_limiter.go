package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// slidingWindowScript implements a sliding-window counter using a Redis sorted set.
// KEYS[1] = rate limit key
// ARGV[1] = limit (max count)
// ARGV[2] = window in milliseconds
// ARGV[3] = cost (units to consume; 1 for RPM, N for TPM)
// ARGV[4] = current time in milliseconds (Unix)
//
// Returns: {allowed (1/0), remaining, window_start_ms}
var slidingWindowScript = redis.NewScript(`
local key     = KEYS[1]
local limit   = tonumber(ARGV[1])
local window  = tonumber(ARGV[2])
local cost    = tonumber(ARGV[3])
local now     = tonumber(ARGV[4])

-- Remove entries outside the window.
redis.call('ZREMRANGEBYSCORE', key, 0, now - window)

local current = redis.call('ZCARD', key)

if current + cost <= limit then
    -- Use a unique member: now + math.random to avoid collisions.
    local member = tostring(now) .. ':' .. tostring(math.random(1, 1000000))
    redis.call('ZADD', key, now, member)
    redis.call('EXPIRE', key, math.ceil(window / 1000) + 1)
    return {1, limit - current - cost, 0}
else
    -- Return the score of the oldest entry so callers can compute Retry-After.
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local reset = 0
    if #oldest > 0 then
        reset = tonumber(oldest[2]) + window
    end
    return {0, 0, reset}
end
`)

// RedisLimiter implements Limiter using Redis sorted-set sliding windows.
type RedisLimiter struct {
	client *redis.Client
}

// NewRedisLimiter creates a RedisLimiter backed by the given client.
func NewRedisLimiter(client *redis.Client) *RedisLimiter {
	return &RedisLimiter{client: client}
}

// Allow checks and (if allowed) increments the sliding-window counter for key.
func (r *RedisLimiter) Allow(ctx context.Context, key string, limit, cost int, window time.Duration) (Result, error) {
	nowMs := time.Now().UnixMilli()
	windowMs := window.Milliseconds()

	vals, err := slidingWindowScript.Run(ctx, r.client,
		[]string{key},
		strconv.Itoa(limit),
		strconv.FormatInt(windowMs, 10),
		strconv.Itoa(cost),
		strconv.FormatInt(nowMs, 10),
	).Slice()
	if err != nil {
		return Result{}, fmt.Errorf("redis rate limit script: %w", err)
	}

	allowed := vals[0].(int64) == 1
	remaining := int(vals[1].(int64))

	var resetAt time.Time
	if resetMs, ok := vals[2].(int64); ok && resetMs > 0 {
		resetAt = time.UnixMilli(resetMs)
	}

	return Result{
		Allowed:   allowed,
		Remaining: remaining,
		ResetAt:   resetAt,
	}, nil
}

// KeyForKeyRPM returns the Redis key for Virtual Key RPM tracking.
func KeyForKeyRPM(keyID, windowMinute string) string {
	return fmt.Sprintf("rl:key:%s:rpm:%s", keyID, windowMinute)
}

// KeyForKeyTPM returns the Redis key for Virtual Key TPM tracking.
func KeyForKeyTPM(keyID, windowMinute string) string {
	return fmt.Sprintf("rl:key:%s:tpm:%s", keyID, windowMinute)
}

// WindowMinute returns a string representing the current minute bucket (YYYYMMDDHHMM).
func WindowMinute() string {
	return time.Now().UTC().Format("200601021504")
}
