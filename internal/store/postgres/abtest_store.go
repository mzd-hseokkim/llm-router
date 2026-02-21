package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/llm-router/gateway/internal/abtest"
)

// ABTestStore persists A/B test experiments and per-request results.
type ABTestStore struct {
	pool *pgxpool.Pool
}

// NewABTestStore returns an ABTestStore backed by pool.
func NewABTestStore(pool *pgxpool.Pool) *ABTestStore {
	return &ABTestStore{pool: pool}
}

// Create inserts a new experiment and populates its ID and timestamps.
func (s *ABTestStore) Create(ctx context.Context, exp *abtest.Experiment) error {
	split, _ := json.Marshal(exp.TrafficSplit)
	target, _ := json.Marshal(exp.Target)
	return s.pool.QueryRow(ctx, `
		INSERT INTO ab_tests
		    (name, status, traffic_split, target, success_metrics,
		     min_samples, confidence_level, start_at, end_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, created_at, updated_at`,
		exp.Name, string(exp.Status), split, target,
		exp.SuccessMetrics, exp.MinSamples, exp.ConfidenceLevel,
		exp.StartAt, exp.EndAt,
	).Scan(&exp.ID, &exp.CreatedAt, &exp.UpdatedAt)
}

// Get returns the experiment with the given ID.
func (s *ABTestStore) Get(ctx context.Context, id string) (*abtest.Experiment, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, status, traffic_split, target, success_metrics,
		       min_samples, confidence_level, start_at, end_at,
		       COALESCE(winner,''), created_at, updated_at
		FROM ab_tests WHERE id = $1`, id)
	exp, err := scanExperiment(row)
	if err != nil {
		return nil, fmt.Errorf("abtest_store get: %w", err)
	}
	return exp, nil
}

// List returns all experiments ordered by creation time descending.
func (s *ABTestStore) List(ctx context.Context) ([]*abtest.Experiment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, status, traffic_split, target, success_metrics,
		       min_samples, confidence_level, start_at, end_at,
		       COALESCE(winner,''), created_at, updated_at
		FROM ab_tests ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("abtest_store list: %w", err)
	}
	defer rows.Close()

	var result []*abtest.Experiment
	for rows.Next() {
		exp, err := scanExperiment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, exp)
	}
	return result, rows.Err()
}

// UpdateStatus transitions an experiment to a new status and (optionally) records a winner.
func (s *ABTestStore) UpdateStatus(ctx context.Context, id, status, winner string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE ab_tests SET status=$1, winner=NULLIF($2,''), updated_at=NOW() WHERE id=$3`,
		status, winner, id)
	if err != nil {
		return fmt.Errorf("abtest_store update_status: %w", err)
	}
	return nil
}

// InsertResult persists one request's metrics.
// ON CONFLICT DO NOTHING protects against duplicate inserts on retry.
func (s *ABTestStore) InsertResult(ctx context.Context, r abtest.Result) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ab_test_results
		    (test_id, variant, request_id, timestamp, model, latency_ms,
		     prompt_tokens, completion_tokens, cost_usd, error, finish_reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (test_id, request_id) DO NOTHING`,
		r.TestID, r.Variant, r.RequestID, r.Timestamp, r.Model,
		r.LatencyMs, r.PromptTokens, r.CompletionTokens, r.CostUSD,
		r.Error, r.FinishReason,
	)
	if err != nil {
		return fmt.Errorf("abtest_store insert_result: %w", err)
	}
	return nil
}

// GetVariantStats aggregates metrics for a single variant.
func (s *ABTestStore) GetVariantStats(ctx context.Context, testID, variant string) (*abtest.VariantStats, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT latency_ms, cost_usd, error
		FROM ab_test_results
		WHERE test_id=$1 AND variant=$2`, testID, variant)
	if err != nil {
		return nil, fmt.Errorf("abtest_store get_stats: %w", err)
	}
	defer rows.Close()

	stats := &abtest.VariantStats{Variant: variant}
	var latencies []float64
	var totalCost float64

	for rows.Next() {
		var latMs int
		var costUSD float64
		var isErr bool
		if err := rows.Scan(&latMs, &costUSD, &isErr); err != nil {
			return nil, err
		}
		stats.Samples++
		latencies = append(latencies, float64(latMs))
		totalCost += costUSD
		if isErr {
			stats.ErrorCount++
		}
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	if stats.Samples > 0 {
		total := 0.0
		for _, l := range latencies {
			total += l
		}
		stats.AvgLatencyMs = total / float64(stats.Samples)
		stats.LatencyP95Ms = abtest.P95(latencies)
		stats.AvgCostPerReq = totalCost / float64(stats.Samples)
		stats.ErrorRate = float64(stats.ErrorCount) / float64(stats.Samples)
		stats.LatencyValues = latencies
	}
	return stats, nil
}

// scanExperiment reads one row from a row/rows source.
// Accepts both pgx.Row (from QueryRow) and pgx.Rows (from Query loop).
func scanExperiment(r interface{ Scan(...any) error }) (*abtest.Experiment, error) {
	var (
		splitJSON []byte
		targetJSON []byte
		metrics   []string
		statusStr string
	)
	exp := &abtest.Experiment{}
	if err := r.Scan(
		&exp.ID, &exp.Name, &statusStr, &splitJSON, &targetJSON,
		&metrics, &exp.MinSamples, &exp.ConfidenceLevel,
		&exp.StartAt, &exp.EndAt, &exp.Winner, &exp.CreatedAt, &exp.UpdatedAt,
	); err != nil {
		return nil, err
	}
	exp.Status = abtest.Status(statusStr)
	exp.SuccessMetrics = metrics
	_ = json.Unmarshal(splitJSON, &exp.TrafficSplit)
	_ = json.Unmarshal(targetJSON, &exp.Target)
	return exp, nil
}
