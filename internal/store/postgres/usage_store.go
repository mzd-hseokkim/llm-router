package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sentinelKeyID is the UUID used in daily_usage when virtual_key_id is unknown.
var sentinelKeyID = uuid.MustParse("00000000-0000-0000-0000-000000000000")

// UsageSummary holds aggregated usage statistics.
type UsageSummary struct {
	TotalRequests    int64
	TotalTokens      int64
	TotalCostUSD     float64
	PromptTokens     int64
	CompletionTokens int64
	ErrorCount       int64
}

// ModelBreakdown holds per-model usage.
type ModelBreakdown struct {
	Model            string
	Provider         string
	RequestCount     int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	CostUSD          float64
}

// DailyBreakdown holds per-day usage.
type DailyBreakdown struct {
	Date         time.Time
	RequestCount int64
	TotalTokens  int64
	CostUSD      float64
}

// UsageStore provides read/write access to the daily_usage table.
type UsageStore struct {
	pool *pgxpool.Pool
}

// NewUsageStore creates a UsageStore.
func NewUsageStore(pool *pgxpool.Pool) *UsageStore {
	return &UsageStore{pool: pool}
}

// dailyUsageRow is used internally for batch upserts.
type dailyUsageRow struct {
	Date             time.Time
	Model            string
	Provider         string
	VirtualKeyID     uuid.UUID
	UserID           *uuid.UUID
	TeamID           *uuid.UUID
	OrgID            *uuid.UUID
	RequestCount     int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CostUSD          float64
	ErrorCount       int
}

// BatchUpsert aggregates the given rows into daily_usage using ON CONFLICT.
func (s *UsageStore) BatchUpsert(ctx context.Context, rows []dailyUsageRow) error {
	if len(rows) == 0 {
		return nil
	}

	const q = `
		INSERT INTO daily_usage (
			date, model, provider, virtual_key_id,
			user_id, team_id, org_id,
			request_count, prompt_tokens, completion_tokens, total_tokens, cost_usd, error_count
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (date, model, provider, virtual_key_id) DO UPDATE SET
			request_count     = daily_usage.request_count     + EXCLUDED.request_count,
			prompt_tokens     = daily_usage.prompt_tokens     + EXCLUDED.prompt_tokens,
			completion_tokens = daily_usage.completion_tokens + EXCLUDED.completion_tokens,
			total_tokens      = daily_usage.total_tokens      + EXCLUDED.total_tokens,
			cost_usd          = daily_usage.cost_usd          + EXCLUDED.cost_usd,
			error_count       = daily_usage.error_count       + EXCLUDED.error_count`

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(q,
			r.Date, r.Model, r.Provider,
			uuidToParam(r.VirtualKeyID),
			uuidPtrToParam(r.UserID), uuidPtrToParam(r.TeamID), uuidPtrToParam(r.OrgID),
			r.RequestCount, r.PromptTokens, r.CompletionTokens, r.TotalTokens,
			r.CostUSD, r.ErrorCount,
		)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("daily_usage upsert: %w", err)
		}
	}
	return nil
}

func uuidToParam(id uuid.UUID) interface{} {
	return id
}

// GetSummary returns aggregated usage for an entity over a date range.
func (s *UsageStore) GetSummary(ctx context.Context, entityType string, entityID uuid.UUID, from, to time.Time) (*UsageSummary, error) {
	var q string
	var args []interface{}

	switch entityType {
	case "key":
		q = `SELECT COALESCE(SUM(request_count),0), COALESCE(SUM(total_tokens),0),
			COALESCE(SUM(cost_usd),0), COALESCE(SUM(prompt_tokens),0),
			COALESCE(SUM(completion_tokens),0), COALESCE(SUM(error_count),0)
			FROM daily_usage WHERE virtual_key_id = $1 AND date >= $2 AND date <= $3`
		args = []interface{}{entityID, from, to}
	case "team":
		q = `SELECT COALESCE(SUM(request_count),0), COALESCE(SUM(total_tokens),0),
			COALESCE(SUM(cost_usd),0), COALESCE(SUM(prompt_tokens),0),
			COALESCE(SUM(completion_tokens),0), COALESCE(SUM(error_count),0)
			FROM daily_usage WHERE team_id = $1 AND date >= $2 AND date <= $3`
		args = []interface{}{entityID, from, to}
	default:
		return nil, fmt.Errorf("unsupported entity type: %q", entityType)
	}

	s2 := &UsageSummary{}
	err := s.pool.QueryRow(ctx, q, args...).Scan(
		&s2.TotalRequests, &s2.TotalTokens, &s2.TotalCostUSD,
		&s2.PromptTokens, &s2.CompletionTokens, &s2.ErrorCount,
	)
	if err != nil {
		return nil, fmt.Errorf("usage summary: %w", err)
	}
	return s2, nil
}

// TopSpender holds aggregated spend for a single virtual key.
type TopSpender struct {
	VirtualKeyID string  `json:"virtual_key_id"`
	RequestCount int64   `json:"request_count"`
	TotalTokens  int64   `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// TopSpenders returns the top N virtual keys by cost over a date range.
func (s *UsageStore) TopSpenders(ctx context.Context, from, to time.Time, limit int) ([]TopSpender, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	const q = `
		SELECT virtual_key_id::text, SUM(request_count), SUM(total_tokens), SUM(cost_usd)
		FROM daily_usage
		WHERE date >= $1 AND date <= $2
		  AND virtual_key_id != '00000000-0000-0000-0000-000000000000'
		GROUP BY virtual_key_id
		ORDER BY SUM(cost_usd) DESC
		LIMIT $3`

	rows, err := s.pool.Query(ctx, q, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("top spenders: %w", err)
	}
	defer rows.Close()

	var out []TopSpender
	for rows.Next() {
		var ts TopSpender
		if err := rows.Scan(&ts.VirtualKeyID, &ts.RequestCount, &ts.TotalTokens, &ts.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, ts)
	}
	return out, rows.Err()
}

// GetByModel returns per-model breakdown for an entity over a date range.
func (s *UsageStore) GetByModel(ctx context.Context, entityType string, entityID uuid.UUID, from, to time.Time) ([]ModelBreakdown, error) {
	var q string
	var args []interface{}

	switch entityType {
	case "key":
		q = `SELECT model, provider,
			SUM(request_count), SUM(prompt_tokens), SUM(completion_tokens), SUM(total_tokens), SUM(cost_usd)
			FROM daily_usage WHERE virtual_key_id = $1 AND date >= $2 AND date <= $3
			GROUP BY model, provider ORDER BY SUM(cost_usd) DESC`
		args = []interface{}{entityID, from, to}
	case "team":
		q = `SELECT model, provider,
			SUM(request_count), SUM(prompt_tokens), SUM(completion_tokens), SUM(total_tokens), SUM(cost_usd)
			FROM daily_usage WHERE team_id = $1 AND date >= $2 AND date <= $3
			GROUP BY model, provider ORDER BY SUM(cost_usd) DESC`
		args = []interface{}{entityID, from, to}
	default:
		return nil, fmt.Errorf("unsupported entity type: %q", entityType)
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("usage by model: %w", err)
	}
	defer rows.Close()

	var out []ModelBreakdown
	for rows.Next() {
		var m ModelBreakdown
		if err := rows.Scan(&m.Model, &m.Provider, &m.RequestCount, &m.PromptTokens, &m.CompletionTokens, &m.TotalTokens, &m.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
