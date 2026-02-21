package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/llm-router/gateway/internal/guardrail"
)

// ErrGuardrailPolicyNotFound is returned when the requested policy does not exist.
var ErrGuardrailPolicyNotFound = fmt.Errorf("guardrail policy not found")

// GuardrailPolicyStore implements guardrail.PolicyStore using PostgreSQL.
type GuardrailPolicyStore struct {
	pool *pgxpool.Pool
}

// NewGuardrailPolicyStore creates a new store backed by the given connection pool.
func NewGuardrailPolicyStore(pool *pgxpool.Pool) *GuardrailPolicyStore {
	return &GuardrailPolicyStore{pool: pool}
}

// List returns all guardrail policies ordered by sort_order, then guardrail_type.
func (s *GuardrailPolicyStore) List(ctx context.Context) ([]*guardrail.PolicyRecord, error) {
	const q = `
		SELECT id, guardrail_type, is_enabled, action, engine, config_json, sort_order, created_at, updated_at
		FROM guardrail_policies
		ORDER BY sort_order, guardrail_type`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list guardrail policies: %w", err)
	}
	defer rows.Close()

	var recs []*guardrail.PolicyRecord
	for rows.Next() {
		rec, err := scanGuardrailPolicy(rows)
		if err != nil {
			return nil, fmt.Errorf("scan guardrail policy: %w", err)
		}
		recs = append(recs, rec)
	}
	return recs, rows.Err()
}

// GetByType retrieves a single policy by its guardrail_type.
func (s *GuardrailPolicyStore) GetByType(ctx context.Context, guardrailType string) (*guardrail.PolicyRecord, error) {
	const q = `
		SELECT id, guardrail_type, is_enabled, action, engine, config_json, sort_order, created_at, updated_at
		FROM guardrail_policies WHERE guardrail_type = $1`

	row := s.pool.QueryRow(ctx, q, guardrailType)
	rec, err := scanGuardrailPolicy(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGuardrailPolicyNotFound
	}
	return rec, err
}

// Upsert inserts or updates a single guardrail policy.
func (s *GuardrailPolicyStore) Upsert(ctx context.Context, rec *guardrail.PolicyRecord) error {
	const q = `
		INSERT INTO guardrail_policies (guardrail_type, is_enabled, action, engine, config_json, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (guardrail_type) DO UPDATE SET
			is_enabled  = EXCLUDED.is_enabled,
			action      = EXCLUDED.action,
			engine      = EXCLUDED.engine,
			config_json = EXCLUDED.config_json,
			sort_order  = EXCLUDED.sort_order,
			updated_at  = NOW()
		RETURNING id, created_at, updated_at`

	var id pgtype.UUID
	var createdAt, updatedAt pgtype.Timestamptz
	err := s.pool.QueryRow(ctx, q,
		rec.GuardrailType, rec.IsEnabled, rec.Action,
		nullableString(rec.Engine), rec.ConfigJSON, rec.SortOrder,
	).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return fmt.Errorf("upsert guardrail policy: %w", err)
	}
	if id.Valid {
		rec.ID = uuid.UUID(id.Bytes).String()
	}
	if createdAt.Valid {
		rec.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		rec.UpdatedAt = updatedAt.Time
	}
	return nil
}

// UpsertAll upserts all given policies in a single transaction.
func (s *GuardrailPolicyStore) UpsertAll(ctx context.Context, recs []*guardrail.PolicyRecord) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	const q = `
		INSERT INTO guardrail_policies (guardrail_type, is_enabled, action, engine, config_json, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (guardrail_type) DO UPDATE SET
			is_enabled  = EXCLUDED.is_enabled,
			action      = EXCLUDED.action,
			engine      = EXCLUDED.engine,
			config_json = EXCLUDED.config_json,
			sort_order  = EXCLUDED.sort_order,
			updated_at  = NOW()
		RETURNING id, created_at, updated_at`

	for _, rec := range recs {
		var id pgtype.UUID
		var createdAt, updatedAt pgtype.Timestamptz
		err := tx.QueryRow(ctx, q,
			rec.GuardrailType, rec.IsEnabled, rec.Action,
			nullableString(rec.Engine), rec.ConfigJSON, rec.SortOrder,
		).Scan(&id, &createdAt, &updatedAt)
		if err != nil {
			return fmt.Errorf("upsert guardrail policy %s: %w", rec.GuardrailType, err)
		}
		if id.Valid {
			rec.ID = uuid.UUID(id.Bytes).String()
		}
		if createdAt.Valid {
			rec.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			rec.UpdatedAt = updatedAt.Time
		}
	}
	return tx.Commit(ctx)
}

// --- helpers ---

func scanGuardrailPolicy(s scanner) (*guardrail.PolicyRecord, error) {
	var (
		rec        guardrail.PolicyRecord
		id         pgtype.UUID
		engine     pgtype.Text
		configJSON pgtype.Text
		createdAt  pgtype.Timestamptz
		updatedAt  pgtype.Timestamptz
	)

	err := s.Scan(
		&id, &rec.GuardrailType, &rec.IsEnabled, &rec.Action,
		&engine, &configJSON, &rec.SortOrder, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if id.Valid {
		rec.ID = uuid.UUID(id.Bytes).String()
	}
	if engine.Valid {
		rec.Engine = engine.String
	}
	if configJSON.Valid {
		rec.ConfigJSON = []byte(configJSON.String)
	}
	if createdAt.Valid {
		rec.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		rec.UpdatedAt = updatedAt.Time
	}
	return &rec, nil
}
