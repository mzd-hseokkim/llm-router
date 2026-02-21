package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/llm-router/gateway/internal/budget"
)

// BudgetStore implements budget.Store using PostgreSQL.
type BudgetStore struct {
	pool *pgxpool.Pool
}

// NewBudgetStore creates a BudgetStore backed by pool.
func NewBudgetStore(pool *pgxpool.Pool) *BudgetStore {
	return &BudgetStore{pool: pool}
}

const budgetSelectCols = `
	id, entity_type, entity_id, period,
	soft_limit_usd, hard_limit_usd, current_spend,
	period_start, period_end`

func scanBudget(row pgx.Row) (*budget.Budget, error) {
	b := &budget.Budget{}
	err := row.Scan(
		&b.ID, &b.EntityType, &b.EntityID, &b.Period,
		&b.SoftLimitUSD, &b.HardLimitUSD, &b.CurrentSpend,
		&b.PeriodStart, &b.PeriodEnd,
	)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Get returns the active budget for (entityType, entityID, period).
func (s *BudgetStore) Get(ctx context.Context, entityType string, entityID uuid.UUID, period string) (*budget.Budget, error) {
	q := `SELECT ` + budgetSelectCols + ` FROM budgets
		WHERE entity_type = $1 AND entity_id = $2 AND period = $3`
	b, err := scanBudget(s.pool.QueryRow(ctx, q, entityType, entityID, period))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("budget get: %w", err)
	}
	return b, nil
}

// List returns all budgets for an entity.
func (s *BudgetStore) List(ctx context.Context, entityType string, entityID uuid.UUID) ([]*budget.Budget, error) {
	q := `SELECT ` + budgetSelectCols + ` FROM budgets
		WHERE entity_type = $1 AND entity_id = $2
		ORDER BY period`
	rows, err := s.pool.Query(ctx, q, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("budget list: %w", err)
	}
	defer rows.Close()

	var out []*budget.Budget
	for rows.Next() {
		b, err := scanBudget(rows)
		if err != nil {
			return nil, fmt.Errorf("budget list scan: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Upsert inserts or updates a budget record.
func (s *BudgetStore) Upsert(ctx context.Context, b *budget.Budget) error {
	q := `
		INSERT INTO budgets (entity_type, entity_id, period, soft_limit_usd, hard_limit_usd, current_spend, period_start, period_end)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (entity_type, entity_id, period) DO UPDATE SET
			soft_limit_usd = EXCLUDED.soft_limit_usd,
			hard_limit_usd = EXCLUDED.hard_limit_usd,
			period_start   = EXCLUDED.period_start,
			period_end     = EXCLUDED.period_end,
			updated_at     = NOW()
		RETURNING id`
	return s.pool.QueryRow(ctx, q,
		b.EntityType, b.EntityID, b.Period,
		b.SoftLimitUSD, b.HardLimitUSD, b.CurrentSpend,
		b.PeriodStart, b.PeriodEnd,
	).Scan(&b.ID)
}

// AddSpend atomically increments current_spend for the given budget.
func (s *BudgetStore) AddSpend(ctx context.Context, entityType string, entityID uuid.UUID, period string, amountUSD float64) error {
	q := `
		UPDATE budgets SET current_spend = current_spend + $4, updated_at = NOW()
		WHERE entity_type = $1 AND entity_id = $2 AND period = $3`
	_, err := s.pool.Exec(ctx, q, entityType, entityID, period, amountUSD)
	if err != nil {
		return fmt.Errorf("budget add spend: %w", err)
	}
	return nil
}

// ResetExpired sets current_spend = 0 and advances period_start/period_end for
// all budgets whose period_end has passed.
func (s *BudgetStore) ResetExpired(ctx context.Context, now time.Time) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, entity_type, entity_id, period
		FROM budgets
		WHERE period != 'lifetime' AND period_end <= $1`, now)
	if err != nil {
		return 0, fmt.Errorf("budget reset query: %w", err)
	}
	defer rows.Close()

	type row struct {
		id         uuid.UUID
		entityType string
		entityID   uuid.UUID
		period     string
	}
	var expired []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.entityType, &r.entityID, &r.period); err != nil {
			return 0, err
		}
		expired = append(expired, r)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	count := 0
	for _, r := range expired {
		start, err1 := budget.PeriodStart(r.period, now)
		end, err2 := budget.PeriodEnd(r.period, now)
		if err1 != nil || err2 != nil {
			continue
		}
		_, err := s.pool.Exec(ctx, `
			UPDATE budgets SET current_spend = 0, period_start = $1, period_end = $2, updated_at = NOW()
			WHERE id = $3`,
			start, end, r.id)
		if err == nil {
			count++
		}
	}
	return count, nil
}
