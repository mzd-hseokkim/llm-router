package exact

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/llm-router/gateway/internal/gateway/types"
)

const keyPrefix = "cache:exact:"

// ErrResponseTooLarge is returned when a response exceeds the max size limit.
var ErrResponseTooLarge = errors.New("response too large for cache")

// Entry is the value stored in the cache.
type Entry struct {
	Response         *types.ChatCompletionResponse `json:"response"`
	CreatedAt        int64                         `json:"created_at"`
	Model            string                        `json:"model"`
	PromptTokens     int                           `json:"prompt_tokens"`
	CompletionTokens int                           `json:"completion_tokens"`
	CostUSD          float64                       `json:"cost_usd"`
}

// Cache provides exact-match LLM response caching backed by Redis.
type Cache struct {
	client          *redis.Client
	maxResponseSize int64
	defaultTTL      time.Duration
}

// New creates an exact-match cache.
func New(client *redis.Client, defaultTTL time.Duration, maxResponseSize int64) *Cache {
	return &Cache{
		client:          client,
		maxResponseSize: maxResponseSize,
		defaultTTL:      defaultTTL,
	}
}

// Get retrieves a cached entry. Returns (nil, nil) on cache miss.
func (c *Cache) Get(ctx context.Context, key string) (*Entry, error) {
	data, err := c.client.Get(ctx, keyPrefix+key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cache get: %w", err)
	}

	decoded := maybeDecompress(data)

	var entry Entry
	if err := json.Unmarshal(decoded, &entry); err != nil {
		return nil, fmt.Errorf("cache unmarshal: %w", err)
	}
	return &entry, nil
}

// Store saves a response to the cache. TTL of 0 uses the default.
func (c *Cache) Store(ctx context.Context, key string, resp *types.ChatCompletionResponse, costUSD float64, ttl time.Duration) error {
	if ttl == 0 {
		ttl = c.defaultTTL
	}

	entry := Entry{
		Response:  resp,
		CreatedAt: time.Now().Unix(),
		Model:     resp.Model,
		CostUSD:   costUSD,
	}
	if resp.Usage != nil {
		entry.PromptTokens = resp.Usage.PromptTokens
		entry.CompletionTokens = resp.Usage.CompletionTokens
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}

	if int64(len(data)) > c.maxResponseSize {
		return ErrResponseTooLarge
	}

	// Compress large responses (best effort)
	if compressed, err := gzipCompress(data); err == nil && len(compressed) < len(data) {
		data = compressed
	}

	if err := c.client.Set(ctx, keyPrefix+key, data, ttl).Err(); err != nil {
		return fmt.Errorf("cache set: %w", err)
	}
	return nil
}

// Delete removes a cache entry by hash key.
func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, keyPrefix+key).Err()
}

// DeleteByPattern removes all entries whose Redis key matches a glob pattern.
// Pattern should not include the keyPrefix (it is added automatically).
func (c *Cache) DeleteByPattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, cur, err := c.client.Scan(ctx, cursor, keyPrefix+pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("cache scan: %w", err)
		}
		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("cache del: %w", err)
			}
		}
		cursor = cur
		if cursor == 0 {
			break
		}
	}
	return nil
}

// Age returns the age of a cached entry in seconds.
func Age(entry *Entry) int64 {
	return time.Now().Unix() - entry.CreatedAt
}

func gzipCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func maybeDecompress(data []byte) []byte {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return data
	}
	defer gz.Close()
	out, err := io.ReadAll(gz)
	if err != nil {
		return data
	}
	return out
}
