package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/llm-router/gateway/internal/gateway/types"
)

// VectorEntry represents a row in the semantic_cache table.
type VectorEntry struct {
	ID               string
	Model            string
	PromptText       string
	ResponseJSON     []byte
	PromptTokens     int
	CompletionTokens int
	CostUSD          float64
	HitCount         int
	Similarity       float64 // only set when returned from a search
}

// VectorStore provides semantic cache storage using pgvector.
type VectorStore struct {
	pool *pgxpool.Pool
}

// NewVectorStore creates a VectorStore.
func NewVectorStore(pool *pgxpool.Pool) *VectorStore {
	return &VectorStore{pool: pool}
}

// Search finds the most similar cached entry for the given embedding and model.
// Returns nil if no entry meets the similarity threshold.
func (s *VectorStore) Search(ctx context.Context, model string, embedding []float32, threshold float64) (*VectorEntry, error) {
	// pgvector uses <=> for cosine distance. similarity = 1 - distance.
	const query = `
		SELECT
			id,
			model,
			prompt_text,
			response_json,
			prompt_tokens,
			completion_tokens,
			cost_usd,
			hit_count,
			1 - (embedding <=> $1::vector) AS similarity
		FROM semantic_cache
		WHERE model = $2
		  AND expires_at > NOW()
		ORDER BY embedding <=> $1::vector
		LIMIT 1
	`

	pgVec := float32SliceToPGVector(embedding)

	row := s.pool.QueryRow(ctx, query, pgVec, model)

	var e VectorEntry
	var costUSD *float64
	if err := row.Scan(
		&e.ID, &e.Model, &e.PromptText, &e.ResponseJSON,
		&e.PromptTokens, &e.CompletionTokens, &costUSD, &e.HitCount, &e.Similarity,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("vector search: %w", err)
	}

	if costUSD != nil {
		e.CostUSD = *costUSD
	}

	if e.Similarity < threshold {
		return nil, nil
	}

	return &e, nil
}

// Store inserts a new semantic cache entry.
func (s *VectorStore) Store(ctx context.Context, model string, embedding []float32, promptText string, resp *types.ChatCompletionResponse, costUSD float64, ttl time.Duration) error {
	respJSON, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("vector store marshal: %w", err)
	}

	pgVec := float32SliceToPGVector(embedding)
	expiresAt := time.Now().Add(ttl)

	var promptTokens, completionTokens int
	if resp.Usage != nil {
		promptTokens = resp.Usage.PromptTokens
		completionTokens = resp.Usage.CompletionTokens
	}

	const query = `
		INSERT INTO semantic_cache
			(model, embedding, prompt_text, response_json, prompt_tokens, completion_tokens, cost_usd, expires_at)
		VALUES ($1, $2::vector, $3, $4, $5, $6, $7, $8)
	`
	_, err = s.pool.Exec(ctx, query,
		model, pgVec, promptText, respJSON,
		promptTokens, completionTokens, costUSD, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("vector store insert: %w", err)
	}
	return nil
}

// IncrHitCount increments the hit counter for a cache entry (best-effort).
func (s *VectorStore) IncrHitCount(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE semantic_cache SET hit_count = hit_count + 1 WHERE id = $1`, id)
	return err
}

// DeleteExpired removes expired entries. Call periodically.
func (s *VectorStore) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM semantic_cache WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// float32SliceToPGVector converts a float32 slice to the pgvector text format "[v1,v2,...]".
func float32SliceToPGVector(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	b := make([]byte, 0, len(v)*8+2)
	b = append(b, '[')
	for i, f := range v {
		if i > 0 {
			b = append(b, ',')
		}
		b = fmt.Appendf(b, "%g", f)
	}
	b = append(b, ']')
	return string(b)
}
