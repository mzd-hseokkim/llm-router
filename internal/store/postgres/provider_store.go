package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/llm-router/gateway/internal/provider"
)

// ProviderStore implements provider.ProviderStore using PostgreSQL.
type ProviderStore struct {
	pool *pgxpool.Pool
}

// NewProviderStore creates a new store backed by the given connection pool.
func NewProviderStore(pool *pgxpool.Pool) *ProviderStore {
	return &ProviderStore{pool: pool}
}

// ErrProviderNotFound is returned when a provider with the given ID/name does not exist.
var ErrProviderNotFound = fmt.Errorf("provider not found")

// Create inserts a new provider record.
func (s *ProviderStore) Create(ctx context.Context, rec *provider.ProviderRecord) error {
	const q = `
		INSERT INTO providers (name, adapter_type, display_name, base_url, is_enabled, config_json, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	row := s.pool.QueryRow(ctx, q,
		rec.Name, rec.AdapterType, rec.DisplayName,
		nullableString(rec.BaseURL), rec.IsEnabled,
		nullableBytes(rec.ConfigJSON), rec.SortOrder,
	)

	var id pgtype.UUID
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &createdAt, &updatedAt); err != nil {
		return fmt.Errorf("create provider: %w", err)
	}
	rec.ID = uuid.UUID(id.Bytes)
	if createdAt.Valid {
		rec.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		rec.UpdatedAt = updatedAt.Time
	}
	return nil
}

// GetByID retrieves a single provider by its UUID.
func (s *ProviderStore) GetByID(ctx context.Context, id uuid.UUID) (*provider.ProviderRecord, error) {
	const q = `
		SELECT id, name, adapter_type, display_name, base_url, is_enabled, config_json, sort_order, created_at, updated_at
		FROM providers WHERE id = $1`

	row := s.pool.QueryRow(ctx, q, id)
	rec, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProviderNotFound
	}
	return rec, err
}

// GetByName retrieves a single provider by its name.
func (s *ProviderStore) GetByName(ctx context.Context, name string) (*provider.ProviderRecord, error) {
	const q = `
		SELECT id, name, adapter_type, display_name, base_url, is_enabled, config_json, sort_order, created_at, updated_at
		FROM providers WHERE name = $1`

	row := s.pool.QueryRow(ctx, q, name)
	rec, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProviderNotFound
	}
	return rec, err
}

// List returns all providers ordered by sort_order, then name.
func (s *ProviderStore) List(ctx context.Context) ([]*provider.ProviderRecord, error) {
	const q = `
		SELECT id, name, adapter_type, display_name, base_url, is_enabled, config_json, sort_order, created_at, updated_at
		FROM providers ORDER BY sort_order, name`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer rows.Close()

	var recs []*provider.ProviderRecord
	for rows.Next() {
		rec, err := scanProvider(rows)
		if err != nil {
			return nil, fmt.Errorf("scan provider: %w", err)
		}
		recs = append(recs, rec)
	}
	return recs, rows.Err()
}

// Update replaces mutable fields of an existing provider.
func (s *ProviderStore) Update(ctx context.Context, rec *provider.ProviderRecord) error {
	const q = `
		UPDATE providers SET
			display_name = $1,
			base_url     = $2,
			is_enabled   = $3,
			config_json  = $4,
			sort_order   = $5,
			updated_at   = NOW()
		WHERE id = $6
		RETURNING updated_at`

	row := s.pool.QueryRow(ctx, q,
		rec.DisplayName, nullableString(rec.BaseURL),
		rec.IsEnabled, nullableBytes(rec.ConfigJSON),
		rec.SortOrder, rec.ID,
	)
	var updatedAt pgtype.Timestamptz
	if err := row.Scan(&updatedAt); errors.Is(err, pgx.ErrNoRows) {
		return ErrProviderNotFound
	} else if err != nil {
		return fmt.Errorf("update provider: %w", err)
	}
	if updatedAt.Valid {
		rec.UpdatedAt = updatedAt.Time
	}
	return nil
}

// Delete permanently removes a provider (cascades to models).
func (s *ProviderStore) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM providers WHERE id = $1`
	tag, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProviderNotFound
	}
	return nil
}

// --- helpers ---

func scanProvider(s scanner) (*provider.ProviderRecord, error) {
	var (
		rec       provider.ProviderRecord
		id        pgtype.UUID
		baseURL   pgtype.Text
		configJSON pgtype.Text
		createdAt pgtype.Timestamptz
		updatedAt pgtype.Timestamptz
	)

	err := s.Scan(
		&id, &rec.Name, &rec.AdapterType, &rec.DisplayName,
		&baseURL, &rec.IsEnabled, &configJSON, &rec.SortOrder,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	rec.ID = uuid.UUID(id.Bytes)
	if baseURL.Valid {
		rec.BaseURL = baseURL.String
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

func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
