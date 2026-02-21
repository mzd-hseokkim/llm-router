package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/llm-router/gateway/internal/billing"
)

// BillingStore provides chargeback data from PostgreSQL.
type BillingStore struct {
	pool *pgxpool.Pool
}

// NewBillingStore returns a BillingStore.
func NewBillingStore(pool *pgxpool.Pool) *BillingStore {
	return &BillingStore{pool: pool}
}

// GetMarkupConfig returns the markup config for the given team (or global if nil).
func (s *BillingStore) GetMarkupConfig(ctx context.Context, teamID *string) (*billing.MarkupConfig, error) {
	var query string
	var args []any

	if teamID == nil {
		query = `SELECT id::TEXT, team_id::TEXT, percentage, fixed_usd, COALESCE(cap_usd,0)
		          FROM markup_configs WHERE team_id IS NULL LIMIT 1`
	} else {
		query = `SELECT id::TEXT, team_id::TEXT, percentage, fixed_usd, COALESCE(cap_usd,0)
		          FROM markup_configs WHERE team_id = $1 LIMIT 1`
		args = []any{*teamID}
	}

	cfg := &billing.MarkupConfig{}
	var tid *string
	err := s.pool.QueryRow(ctx, query, args...).
		Scan(&cfg.ID, &tid, &cfg.Percentage, &cfg.FixedUSD, &cfg.CapUSD)
	if err != nil {
		return nil, fmt.Errorf("billing_store get_markup: %w", err)
	}
	cfg.TeamID = tid
	return cfg, nil
}

// UpsertMarkupConfig creates or updates a markup config for a team.
func (s *BillingStore) UpsertMarkupConfig(ctx context.Context, cfg *billing.MarkupConfig) error {
	if cfg.TeamID == nil {
		_, err := s.pool.Exec(ctx,
			`UPDATE markup_configs SET percentage=$1, fixed_usd=$2, cap_usd=NULLIF($3,0), updated_at=NOW()
			 WHERE team_id IS NULL`,
			cfg.Percentage, cfg.FixedUSD, cfg.CapUSD)
		return err
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO markup_configs (team_id, percentage, fixed_usd, cap_usd)
		 VALUES ($1, $2, $3, NULLIF($4,0))
		 ON CONFLICT (team_id) DO UPDATE
		   SET percentage=EXCLUDED.percentage, fixed_usd=EXCLUDED.fixed_usd,
		       cap_usd=EXCLUDED.cap_usd, updated_at=NOW()`,
		*cfg.TeamID, cfg.Percentage, cfg.FixedUSD, cfg.CapUSD)
	return err
}

// GetTeamUsage returns aggregated usage per team from daily_usage for the given range.
func (s *BillingStore) GetTeamUsage(ctx context.Context, from, to time.Time) ([]*billing.TeamUsage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
		    du.team_id::TEXT,
		    COALESCE(t.name, 'unknown') AS team_name,
		    SUM(du.cost_usd)           AS cost_usd,
		    SUM(du.total_tokens)       AS tokens,
		    SUM(du.request_count)      AS requests
		FROM daily_usage du
		LEFT JOIN teams t ON t.id = du.team_id
		WHERE du.date >= $1 AND du.date < $2
		  AND du.team_id IS NOT NULL
		GROUP BY du.team_id, t.name
		ORDER BY cost_usd DESC`,
		from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("billing_store team_usage: %w", err)
	}
	defer rows.Close()

	var result []*billing.TeamUsage
	for rows.Next() {
		tu := &billing.TeamUsage{}
		if err := rows.Scan(&tu.TeamID, &tu.TeamName, &tu.CostUSD, &tu.Tokens, &tu.Requests); err != nil {
			return nil, err
		}
		result = append(result, tu)
	}
	return result, rows.Err()
}

// GetModelBreakdown returns per-model cost for a team.
func (s *BillingStore) GetModelBreakdown(ctx context.Context, teamID string, from, to time.Time) ([]*billing.ModelBreakdown, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT model, SUM(cost_usd), SUM(total_tokens)
		FROM daily_usage
		WHERE team_id = $1 AND date >= $2 AND date < $3
		GROUP BY model ORDER BY SUM(cost_usd) DESC`,
		teamID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("billing_store model_breakdown: %w", err)
	}
	defer rows.Close()

	var result []*billing.ModelBreakdown
	for rows.Next() {
		mb := &billing.ModelBreakdown{}
		if err := rows.Scan(&mb.Model, &mb.CostUSD, &mb.Tokens); err != nil {
			return nil, err
		}
		result = append(result, mb)
	}
	return result, rows.Err()
}

// GetTagBreakdown returns per-project-tag cost using request_logs metadata.
func (s *BillingStore) GetTagBreakdown(ctx context.Context, teamID string, from, to time.Time) ([]*billing.TagBreakdown, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
		    COALESCE(metadata->>'project', 'untagged') AS tag,
		    SUM(cost_usd)                              AS cost_usd
		FROM request_logs
		WHERE team_id = $1
		  AND timestamp >= $2 AND timestamp < $3
		  AND cost_usd IS NOT NULL
		GROUP BY tag
		ORDER BY cost_usd DESC
		LIMIT 50`,
		teamID, from, to)
	if err != nil {
		return nil, fmt.Errorf("billing_store tag_breakdown: %w", err)
	}
	defer rows.Close()

	var result []*billing.TagBreakdown
	for rows.Next() {
		tb := &billing.TagBreakdown{}
		if err := rows.Scan(&tb.Tag, &tb.CostUSD); err != nil {
			return nil, err
		}
		result = append(result, tb)
	}
	return result, rows.Err()
}

// GetBillingUsageItems returns external-billing-API line items for the given period.
func (s *BillingStore) GetBillingUsageItems(ctx context.Context, from, to time.Time) ([]*billing.BillingUsageItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
		    du.team_id::TEXT,
		    COALESCE(t.name, 'unknown') AS team_name,
		    SUM(du.total_tokens)        AS tokens,
		    SUM(du.cost_usd)            AS amount
		FROM daily_usage du
		LEFT JOIN teams t ON t.id = du.team_id
		WHERE du.date >= $1 AND du.date < $2 AND du.team_id IS NOT NULL
		GROUP BY du.team_id, t.name
		ORDER BY amount DESC`,
		from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("billing_store billing_items: %w", err)
	}
	defer rows.Close()

	const unitPrice = 0.00001 // $0.01 per 1000 tokens (illustrative)
	var items []*billing.BillingUsageItem
	for rows.Next() {
		var teamID, teamName string
		var tokens int64
		var amount float64
		if err := rows.Scan(&teamID, &teamName, &tokens, &amount); err != nil {
			return nil, err
		}
		items = append(items, &billing.BillingUsageItem{
			TeamID:      teamID,
			PeriodStart: from.Format("2006-01-02"),
			PeriodEnd:   to.Format("2006-01-02"),
			Tokens:      tokens,
			UnitPriceUSD: unitPrice,
			AmountUSD:   amount,
			Metadata:    map[string]string{"team_name": teamName},
		})
	}
	return items, rows.Err()
}
