package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/llm-router/gateway/internal/telemetry"
)

// LogStore persists request log entries to PostgreSQL.
type LogStore struct {
	pool *pgxpool.Pool
}

// NewLogStore returns a LogStore backed by the given pool.
func NewLogStore(pool *pgxpool.Pool) *LogStore {
	return &LogStore{pool: pool}
}

// BatchInsert writes a slice of log entries in a single pgx batch round-trip.
func (s *LogStore) BatchInsert(ctx context.Context, entries []*telemetry.LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	const q = `
		INSERT INTO request_logs (
			request_id, timestamp, model, provider,
			virtual_key_id, user_id, team_id, org_id,
			prompt_tokens, completion_tokens, total_tokens,
			cost_usd, latency_ms, ttft_ms,
			status_code, finish_reason, cache_hit, is_streaming,
			error_code, error_message, metadata
		) VALUES (
			$1,  $2,  $3,  $4,
			$5,  $6,  $7,  $8,
			$9,  $10, $11,
			$12, $13, $14,
			$15, $16, $17, $18,
			$19, $20, $21
		)`

	batch := &pgx.Batch{}
	for _, e := range entries {
		meta, _ := json.Marshal(e.Metadata)
		if meta == nil {
			meta = []byte("{}")
		}
		batch.Queue(q,
			e.RequestID, e.Timestamp, e.Model, e.Provider,
			uuidPtrToParam(e.VirtualKeyID), uuidPtrToParam(e.UserID),
			uuidPtrToParam(e.TeamID), uuidPtrToParam(e.OrgID),
			e.PromptTokens, e.CompletionTokens, e.TotalTokens,
			e.CostUSD, e.LatencyMs, e.TTFTMs,
			e.StatusCode, e.FinishReason, e.CacheHit, e.IsStreaming,
			e.ErrorCode, e.ErrorMessage, meta,
		)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range entries {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("batch log insert: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return err
	}

	// Aggregate into daily_usage (best-effort; errors are non-fatal).
	s.upsertDailyUsage(ctx, entries)
	return nil
}

// upsertDailyUsage aggregates entries and upserts into the daily_usage table.
func (s *LogStore) upsertDailyUsage(ctx context.Context, entries []*telemetry.LogEntry) {
	type key struct {
		date         string
		model        string
		provider     string
		virtualKeyID string
	}
	type agg struct {
		date             time.Time
		model            string
		provider         string
		virtualKeyID     *uuid.UUID
		userID           *uuid.UUID
		teamID           *uuid.UUID
		orgID            *uuid.UUID
		requestCount     int
		promptTokens     int
		completionTokens int
		totalTokens      int
		costUSD          float64
		errorCount       int
	}

	sentinel := "00000000-0000-0000-0000-000000000000"
	grouped := make(map[key]*agg)

	for _, e := range entries {
		if e.Model == "" {
			continue
		}
		dateStr := e.Timestamp.UTC().Format(time.DateOnly)
		keyID := sentinel
		if e.VirtualKeyID != nil {
			keyID = e.VirtualKeyID.String()
		}
		k := key{date: dateStr, model: e.Model, provider: e.Provider, virtualKeyID: keyID}

		a, ok := grouped[k]
		if !ok {
			d, _ := time.Parse(time.DateOnly, dateStr)
			a = &agg{
				date:         d,
				model:        e.Model,
				provider:     e.Provider,
				virtualKeyID: e.VirtualKeyID,
				userID:       e.UserID,
				teamID:       e.TeamID,
				orgID:        e.OrgID,
			}
			grouped[k] = a
		}
		a.requestCount++
		a.promptTokens += e.PromptTokens
		a.completionTokens += e.CompletionTokens
		a.totalTokens += e.TotalTokens
		a.costUSD += e.CostUSD
		if e.StatusCode >= 500 || e.ErrorCode != "" {
			a.errorCount++
		}
	}

	if len(grouped) == 0 {
		return
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

	sentinelUUID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	batch := &pgx.Batch{}
	for _, a := range grouped {
		keyID := sentinelUUID
		if a.virtualKeyID != nil {
			keyID = *a.virtualKeyID
		}
		batch.Queue(q,
			a.date, a.model, a.provider, keyID,
			uuidPtrToParam(a.userID), uuidPtrToParam(a.teamID), uuidPtrToParam(a.orgID),
			a.requestCount, a.promptTokens, a.completionTokens, a.totalTokens,
			a.costUSD, a.errorCount,
		)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range grouped {
		br.Exec() //nolint:errcheck — best-effort
	}
}

// GetByRequestID returns a single log entry by its request_id string.
func (s *LogStore) GetByRequestID(ctx context.Context, requestID string) (*telemetry.LogEntry, error) {
	const q = `
		SELECT
			request_id, timestamp, model, provider,
			virtual_key_id, user_id, team_id, org_id,
			prompt_tokens, completion_tokens, total_tokens,
			cost_usd, latency_ms, ttft_ms,
			status_code, finish_reason, cache_hit, is_streaming,
			error_code, error_message, metadata
		FROM request_logs
		WHERE request_id = $1`

	row := s.pool.QueryRow(ctx, q, requestID)
	e, err := scanLogEntry(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLogNotFound
	}
	return e, err
}

// ErrLogNotFound is returned when a log entry is not found.
var ErrLogNotFound = errors.New("log entry not found")

// CacheStats returns the total number of cache lookups and cache hits for the
// given time range, queried directly from request_logs.
func (s *LogStore) CacheStats(ctx context.Context, from, to time.Time) (total, hits int64, err error) {
	const q = `
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE cache_hit = true) AS hits
		FROM request_logs
		WHERE timestamp >= $1 AND timestamp <= $2`
	row := s.pool.QueryRow(ctx, q, from, to)
	err = row.Scan(&total, &hits)
	return
}

// LogFilter specifies optional filter parameters for listing log entries.
type LogFilter struct {
	VirtualKeyID *uuid.UUID
	From         time.Time
	To           time.Time
	Limit        int
	Offset       int
}

// List returns log entries ordered by timestamp descending, applying any filters.
func (s *LogStore) List(ctx context.Context, f LogFilter) ([]*telemetry.LogEntry, error) {
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}
	if f.From.IsZero() {
		f.From = time.Now().AddDate(0, 0, -7)
	}
	if f.To.IsZero() {
		f.To = time.Now()
	}

	args := []any{f.From, f.To, f.Limit, f.Offset}
	where := "WHERE timestamp >= $1 AND timestamp <= $2"

	if f.VirtualKeyID != nil {
		args = append(args, pgtype.UUID{Bytes: *f.VirtualKeyID, Valid: true})
		where += fmt.Sprintf(" AND virtual_key_id = $%d", len(args))
	}

	q := fmt.Sprintf(`
		SELECT
			request_id, timestamp, model, provider,
			virtual_key_id, user_id, team_id, org_id,
			prompt_tokens, completion_tokens, total_tokens,
			cost_usd, latency_ms, ttft_ms,
			status_code, finish_reason, cache_hit, is_streaming,
			error_code, error_message, metadata
		FROM request_logs
		%s
		ORDER BY timestamp DESC
		LIMIT $3 OFFSET $4`, where)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list request logs: %w", err)
	}
	defer rows.Close()

	var entries []*telemetry.LogEntry
	for rows.Next() {
		e, err := scanLogEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan log entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Count returns the total number of log entries matching the filter (used for pagination).
func (s *LogStore) Count(ctx context.Context, f LogFilter) (int64, error) {
	if f.From.IsZero() {
		f.From = time.Now().AddDate(0, 0, -7)
	}
	if f.To.IsZero() {
		f.To = time.Now()
	}

	args := []any{f.From, f.To}
	where := "WHERE timestamp >= $1 AND timestamp <= $2"

	if f.VirtualKeyID != nil {
		args = append(args, pgtype.UUID{Bytes: *f.VirtualKeyID, Valid: true})
		where += fmt.Sprintf(" AND virtual_key_id = $%d", len(args))
	}

	q := fmt.Sprintf("SELECT COUNT(*) FROM request_logs %s", where)
	var total int64
	err := s.pool.QueryRow(ctx, q, args...).Scan(&total)
	return total, err
}

func scanLogEntry(s scanner) (*telemetry.LogEntry, error) {
	var (
		e              telemetry.LogEntry
		virtualKeyID   pgtype.UUID
		userID         pgtype.UUID
		teamID         pgtype.UUID
		orgID          pgtype.UUID
		promptTokens   pgtype.Int4
		compTokens     pgtype.Int4
		totalTokens    pgtype.Int4
		costUSD        pgtype.Numeric
		latencyMs      pgtype.Int4
		ttftMs         pgtype.Int4
		statusCode     pgtype.Int2
		finishReason   pgtype.Text
		errorCode      pgtype.Text
		errorMessage   pgtype.Text
		metadataBytes  []byte
	)

	err := s.Scan(
		&e.RequestID, &e.Timestamp, &e.Model, &e.Provider,
		&virtualKeyID, &userID, &teamID, &orgID,
		&promptTokens, &compTokens, &totalTokens,
		&costUSD, &latencyMs, &ttftMs,
		&statusCode, &finishReason, &e.CacheHit, &e.IsStreaming,
		&errorCode, &errorMessage, &metadataBytes,
	)
	if err != nil {
		return nil, err
	}

	if virtualKeyID.Valid {
		id := uuid.UUID(virtualKeyID.Bytes)
		e.VirtualKeyID = &id
	}
	if userID.Valid {
		id := uuid.UUID(userID.Bytes)
		e.UserID = &id
	}
	if teamID.Valid {
		id := uuid.UUID(teamID.Bytes)
		e.TeamID = &id
	}
	if orgID.Valid {
		id := uuid.UUID(orgID.Bytes)
		e.OrgID = &id
	}
	if promptTokens.Valid {
		e.PromptTokens = int(promptTokens.Int32)
	}
	if compTokens.Valid {
		e.CompletionTokens = int(compTokens.Int32)
	}
	if totalTokens.Valid {
		e.TotalTokens = int(totalTokens.Int32)
	}
	if costUSD.Valid {
		if f, err := costUSD.Float64Value(); err == nil && f.Valid {
			e.CostUSD = f.Float64
		}
	}
	if latencyMs.Valid {
		e.LatencyMs = int64(latencyMs.Int32)
	}
	if ttftMs.Valid {
		v := int64(ttftMs.Int32)
		e.TTFTMs = &v
	}
	if statusCode.Valid {
		e.StatusCode = int(statusCode.Int16)
	}
	if finishReason.Valid {
		e.FinishReason = finishReason.String
	}
	if errorCode.Valid {
		e.ErrorCode = errorCode.String
	}
	if errorMessage.Valid {
		e.ErrorMessage = errorMessage.String
	}
	if len(metadataBytes) > 0 {
		_ = json.Unmarshal(metadataBytes, &e.Metadata)
	}

	return &e, nil
}
