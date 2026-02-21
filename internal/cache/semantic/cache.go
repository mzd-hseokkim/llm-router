package semantic

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	pgstore "github.com/llm-router/gateway/internal/store/postgres"

	"github.com/llm-router/gateway/internal/gateway/types"
)

// Cache provides semantic response caching backed by pgvector.
type Cache struct {
	store     *pgstore.VectorStore
	embedder  EmbeddingProvider
	threshold float64
	ttl       time.Duration
	logger    *slog.Logger
}

// New creates a semantic cache.
func New(store *pgstore.VectorStore, embedder EmbeddingProvider, threshold float64, ttl time.Duration, logger *slog.Logger) *Cache {
	return &Cache{
		store:     store,
		embedder:  embedder,
		threshold: threshold,
		ttl:       ttl,
		logger:    logger,
	}
}

// Lookup checks for a semantically similar cached response.
// Returns (response, similarity, nil) on hit; (nil, 0, nil) on miss.
// On embedding error the miss is treated as non-fatal.
func (c *Cache) Lookup(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, float64, error) {
	text := buildCacheText(req)
	if text == "" {
		return nil, 0, nil
	}

	embedding, err := c.embedder.Embed(ctx, text)
	if err != nil {
		// Non-fatal: skip semantic cache on embedding failure
		c.logger.Warn("semantic cache embedding failed, skipping", "error", err)
		return nil, 0, nil
	}

	entry, err := c.store.Search(ctx, req.Model, embedding, c.threshold)
	if err != nil {
		c.logger.Warn("semantic cache search failed", "error", err)
		return nil, 0, nil
	}
	if entry == nil {
		return nil, 0, nil
	}

	var resp types.ChatCompletionResponse
	if err := json.Unmarshal(entry.ResponseJSON, &resp); err != nil {
		return nil, 0, fmt.Errorf("semantic cache unmarshal: %w", err)
	}

	// Async hit count increment
	go c.store.IncrHitCount(context.Background(), entry.ID) //nolint:errcheck

	c.logger.Info("semantic cache hit",
		"similarity", entry.Similarity,
		"model", req.Model)

	return &resp, entry.Similarity, nil
}

// Store saves a response and its embedding to the vector store (non-blocking).
func (c *Cache) Store(ctx context.Context, req *types.ChatCompletionRequest, resp *types.ChatCompletionResponse, costUSD float64) {
	text := buildCacheText(req)
	if text == "" {
		return
	}

	go func() {
		storeCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		embedding, err := c.embedder.Embed(storeCtx, text)
		if err != nil {
			c.logger.Warn("semantic cache store: embedding failed", "error", err)
			return
		}

		if err := c.store.Store(storeCtx, req.Model, embedding, text, resp, costUSD, c.ttl); err != nil {
			c.logger.Warn("semantic cache store: db insert failed", "error", err)
		}
	}()
}

// buildCacheText builds the text to embed for cache lookup.
// Uses the last user message, prefixed with the model name.
func buildCacheText(req *types.ChatCompletionRequest) string {
	var lastUser string
	for _, m := range req.Messages {
		if m.Role == "user" {
			lastUser = m.Content
		}
	}
	if lastUser == "" {
		return ""
	}
	return fmt.Sprintf("[model:%s] %s", req.Model, strings.TrimSpace(lastUser))
}
