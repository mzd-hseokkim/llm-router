package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/llm-router/gateway/internal/provider"
)

// ProviderKeyStore implements provider.ProviderKeyStore using PostgreSQL.
type ProviderKeyStore struct {
	pool *pgxpool.Pool
}

// NewProviderKeyStore creates a new store backed by the given connection pool.
func NewProviderKeyStore(pool *pgxpool.Pool) *ProviderKeyStore {
	return &ProviderKeyStore{pool: pool}
}

// ErrProviderKeyNotFound is returned when a key with the given ID does not exist.
var ErrProviderKeyNotFound = fmt.Errorf("provider key not found")

// Create inserts a new provider key record.
func (s *ProviderKeyStore) Create(ctx context.Context, rec *provider.ProviderKeyRecord) error {
	const q = `
		INSERT INTO provider_keys (
			provider, key_alias, encrypted_key, key_preview,
			group_name, tags, is_active, weight,
			monthly_budget_usd
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`

	row := s.pool.QueryRow(ctx, q,
		rec.Provider, rec.KeyAlias, rec.EncryptedKey, rec.KeyPreview,
		nullableString(rec.GroupName), rec.Tags, rec.IsActive, rec.Weight,
		rec.MonthlyBudgetUSD,
	)
	return row.Scan(&rec.ID, &rec.CreatedAt, &rec.UpdatedAt)
}

// GetByID retrieves a single provider key by its UUID.
func (s *ProviderKeyStore) GetByID(ctx context.Context, id uuid.UUID) (*provider.ProviderKeyRecord, error) {
	const q = `
		SELECT id, provider, key_alias, encrypted_key, key_preview,
		       group_name, tags, is_active, weight,
		       monthly_budget_usd, current_month_spend,
		       created_at, updated_at, last_used_at, use_count
		FROM provider_keys WHERE id = $1`

	row := s.pool.QueryRow(ctx, q, id)
	rec, err := scanProviderKey(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProviderKeyNotFound
	}
	return rec, err
}

// List returns all provider keys, optionally filtered by provider name.
func (s *ProviderKeyStore) List(ctx context.Context, providerFilter string) ([]*provider.ProviderKeyRecord, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if providerFilter == "" {
		const q = `SELECT id, provider, key_alias, encrypted_key, key_preview,
			group_name, tags, is_active, weight,
			monthly_budget_usd, current_month_spend,
			created_at, updated_at, last_used_at, use_count
		FROM provider_keys ORDER BY created_at DESC`
		rows, err = s.pool.Query(ctx, q)
	} else {
		const q = `SELECT id, provider, key_alias, encrypted_key, key_preview,
			group_name, tags, is_active, weight,
			monthly_budget_usd, current_month_spend,
			created_at, updated_at, last_used_at, use_count
		FROM provider_keys WHERE provider = $1 ORDER BY created_at DESC`
		rows, err = s.pool.Query(ctx, q, providerFilter)
	}
	if err != nil {
		return nil, fmt.Errorf("list provider keys: %w", err)
	}
	defer rows.Close()

	var recs []*provider.ProviderKeyRecord
	for rows.Next() {
		rec, err := scanProviderKey(rows)
		if err != nil {
			return nil, fmt.Errorf("scan provider key: %w", err)
		}
		recs = append(recs, rec)
	}
	return recs, rows.Err()
}

// ListActive returns all active keys for a provider and optional group.
func (s *ProviderKeyStore) ListActive(ctx context.Context, providerName, groupName string) ([]*provider.ProviderKeyRecord, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if groupName == "" {
		const q = `SELECT id, provider, key_alias, encrypted_key, key_preview,
			group_name, tags, is_active, weight,
			monthly_budget_usd, current_month_spend,
			created_at, updated_at, last_used_at, use_count
		FROM provider_keys WHERE provider = $1 AND is_active = true ORDER BY weight DESC`
		rows, err = s.pool.Query(ctx, q, providerName)
	} else {
		const q = `SELECT id, provider, key_alias, encrypted_key, key_preview,
			group_name, tags, is_active, weight,
			monthly_budget_usd, current_month_spend,
			created_at, updated_at, last_used_at, use_count
		FROM provider_keys WHERE provider = $1 AND group_name = $2 AND is_active = true ORDER BY weight DESC`
		rows, err = s.pool.Query(ctx, q, providerName, groupName)
	}
	if err != nil {
		return nil, fmt.Errorf("list active provider keys: %w", err)
	}
	defer rows.Close()

	var recs []*provider.ProviderKeyRecord
	for rows.Next() {
		rec, err := scanProviderKey(rows)
		if err != nil {
			return nil, fmt.Errorf("scan provider key: %w", err)
		}
		recs = append(recs, rec)
	}
	return recs, rows.Err()
}

// Update replaces mutable fields of an existing provider key (not the encrypted key).
func (s *ProviderKeyStore) Update(ctx context.Context, rec *provider.ProviderKeyRecord) error {
	const q = `
		UPDATE provider_keys SET
			key_alias          = $1,
			group_name         = $2,
			tags               = $3,
			is_active          = $4,
			weight             = $5,
			monthly_budget_usd = $6,
			updated_at         = NOW()
		WHERE id = $7
		RETURNING updated_at`

	row := s.pool.QueryRow(ctx, q,
		rec.KeyAlias, nullableString(rec.GroupName), rec.Tags,
		rec.IsActive, rec.Weight, rec.MonthlyBudgetUSD,
		rec.ID,
	)
	if err := row.Scan(&rec.UpdatedAt); errors.Is(err, pgx.ErrNoRows) {
		return ErrProviderKeyNotFound
	} else if err != nil {
		return fmt.Errorf("update provider key: %w", err)
	}
	return nil
}

// Delete permanently removes a provider key.
func (s *ProviderKeyStore) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM provider_keys WHERE id = $1`
	tag, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete provider key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProviderKeyNotFound
	}
	return nil
}

// RotateKey replaces the encrypted key and preview for the given record.
func (s *ProviderKeyStore) RotateKey(ctx context.Context, id uuid.UUID, encryptedKey []byte, preview string) error {
	const q = `
		UPDATE provider_keys SET
			encrypted_key = $1,
			key_preview   = $2,
			updated_at    = NOW()
		WHERE id = $3`
	tag, err := s.pool.Exec(ctx, q, encryptedKey, preview, id)
	if err != nil {
		return fmt.Errorf("rotate provider key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProviderKeyNotFound
	}
	return nil
}

// --- helpers ---

func scanProviderKey(s scanner) (*provider.ProviderKeyRecord, error) {
	var (
		rec               provider.ProviderKeyRecord
		id                pgtype.UUID
		groupName         pgtype.Text
		monthlyBudget     pgtype.Numeric
		lastUsedAt        pgtype.Timestamptz
		createdAt         pgtype.Timestamptz
		updatedAt         pgtype.Timestamptz
		currentMonthSpend pgtype.Numeric
	)

	err := s.Scan(
		&id, &rec.Provider, &rec.KeyAlias, &rec.EncryptedKey, &rec.KeyPreview,
		&groupName, &rec.Tags, &rec.IsActive, &rec.Weight,
		&monthlyBudget, &currentMonthSpend,
		&createdAt, &updatedAt, &lastUsedAt, &rec.UseCount,
	)
	if err != nil {
		return nil, err
	}

	rec.ID = uuid.UUID(id.Bytes)

	if groupName.Valid {
		rec.GroupName = groupName.String
	}
	if monthlyBudget.Valid {
		f, _ := monthlyBudget.Float64Value()
		if f.Valid {
			v := f.Float64
			rec.MonthlyBudgetUSD = &v
		}
	}
	if currentMonthSpend.Valid {
		f, _ := currentMonthSpend.Float64Value()
		if f.Valid {
			rec.CurrentMonthSpend = f.Float64
		}
	}
	if createdAt.Valid {
		rec.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		rec.UpdatedAt = updatedAt.Time
	}
	if lastUsedAt.Valid {
		t := lastUsedAt.Time
		rec.LastUsedAt = &t
	}

	return &rec, nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ensure time package is used (for the pool helper in the same package)
var _ = time.UTC
