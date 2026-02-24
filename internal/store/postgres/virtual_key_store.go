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
	"github.com/llm-router/gateway/internal/auth"
)

// VirtualKeyStore implements auth.Store using PostgreSQL.
type VirtualKeyStore struct {
	pool *pgxpool.Pool
}

// NewVirtualKeyStore creates a new store backed by the given connection pool.
func NewVirtualKeyStore(pool *pgxpool.Pool) *VirtualKeyStore {
	return &VirtualKeyStore{pool: pool}
}

// Create inserts a new virtual key record. key.ID is populated on success.
func (s *VirtualKeyStore) Create(ctx context.Context, key *auth.VirtualKey) error {
	const q = `
		INSERT INTO virtual_keys (
			key_hash, key_prefix, name,
			user_id, team_id, org_id,
			expires_at, budget_usd, rpm_limit, tpm_limit,
			allowed_models, blocked_models, metadata, is_active
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12, $13, $14
		)
		RETURNING id, created_at, updated_at`

	metadata := key.Metadata
	if metadata == nil {
		metadata = []byte("{}")
	}

	row := s.pool.QueryRow(ctx, q,
		key.KeyHash, key.KeyPrefix, key.Name,
		uuidPtrToParam(key.UserID), uuidPtrToParam(key.TeamID), uuidPtrToParam(key.OrgID),
		key.ExpiresAt, key.BudgetUSD, key.RPMLimit, key.TPMLimit,
		key.AllowedModels, key.BlockedModels, metadata, key.IsActive,
	)
	return row.Scan(&key.ID, &key.CreatedAt, &key.UpdatedAt)
}

// GetByHash looks up a virtual key by prefix (for index hint) then hash.
func (s *VirtualKeyStore) GetByHash(ctx context.Context, keyPrefix, keyHash string) (*auth.VirtualKey, error) {
	const q = `
		SELECT id, key_hash, key_prefix, name,
		       user_id, team_id, org_id,
		       expires_at, budget_usd, rpm_limit, tpm_limit,
		       allowed_models, blocked_models, metadata,
		       is_active, created_at, updated_at, last_used_at
		FROM virtual_keys
		WHERE key_prefix = $1 AND key_hash = $2`

	row := s.pool.QueryRow(ctx, q, keyPrefix, keyHash)
	key, err := scanVirtualKey(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, auth.ErrKeyNotFound
	}
	return key, err
}

// GetByID looks up a virtual key by its UUID.
func (s *VirtualKeyStore) GetByID(ctx context.Context, id uuid.UUID) (*auth.VirtualKey, error) {
	const q = `
		SELECT id, key_hash, key_prefix, name,
		       user_id, team_id, org_id,
		       expires_at, budget_usd, rpm_limit, tpm_limit,
		       allowed_models, blocked_models, metadata,
		       is_active, created_at, updated_at, last_used_at
		FROM virtual_keys
		WHERE id = $1`

	row := s.pool.QueryRow(ctx, q, id)
	key, err := scanVirtualKey(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, auth.ErrKeyNotFound
	}
	return key, err
}

// List returns all virtual keys ordered by creation time (newest first).
func (s *VirtualKeyStore) List(ctx context.Context) ([]*auth.VirtualKey, error) {
	const q = `
		SELECT id, key_hash, key_prefix, name,
		       user_id, team_id, org_id,
		       expires_at, budget_usd, rpm_limit, tpm_limit,
		       allowed_models, blocked_models, metadata,
		       is_active, created_at, updated_at, last_used_at
		FROM virtual_keys
		ORDER BY created_at DESC`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list virtual keys: %w", err)
	}
	defer rows.Close()

	var keys []*auth.VirtualKey
	for rows.Next() {
		key, err := scanVirtualKey(rows)
		if err != nil {
			return nil, fmt.Errorf("scan virtual key: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// ListPage returns a paginated slice of virtual keys ordered by creation time (newest first).
func (s *VirtualKeyStore) ListPage(ctx context.Context, limit, offset int) ([]*auth.VirtualKey, error) {
	const q = `
		SELECT id, key_hash, key_prefix, name,
		       user_id, team_id, org_id,
		       expires_at, budget_usd, rpm_limit, tpm_limit,
		       allowed_models, blocked_models, metadata,
		       is_active, created_at, updated_at, last_used_at
		FROM virtual_keys
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`

	rows, err := s.pool.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list virtual keys page: %w", err)
	}
	defer rows.Close()

	var keys []*auth.VirtualKey
	for rows.Next() {
		key, err := scanVirtualKey(rows)
		if err != nil {
			return nil, fmt.Errorf("scan virtual key: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// CountKeys returns the total number of virtual keys.
func (s *VirtualKeyStore) CountKeys(ctx context.Context) (int64, error) {
	var total int64
	err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM virtual_keys").Scan(&total)
	return total, err
}

// Update replaces all mutable fields of an existing key.
func (s *VirtualKeyStore) Update(ctx context.Context, key *auth.VirtualKey) error {
	const q = `
		UPDATE virtual_keys SET
			name           = $1,
			expires_at     = $2,
			budget_usd     = $3,
			rpm_limit      = $4,
			tpm_limit      = $5,
			allowed_models = $6,
			blocked_models = $7,
			metadata       = $8,
			is_active      = $9,
			updated_at     = NOW()
		WHERE id = $10
		RETURNING updated_at`

	metadata := key.Metadata
	if metadata == nil {
		metadata = []byte("{}")
	}

	row := s.pool.QueryRow(ctx, q,
		key.Name, key.ExpiresAt, key.BudgetUSD, key.RPMLimit, key.TPMLimit,
		key.AllowedModels, key.BlockedModels, metadata, key.IsActive,
		key.ID,
	)
	if err := row.Scan(&key.UpdatedAt); errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrKeyNotFound
	} else if err != nil {
		return fmt.Errorf("update virtual key: %w", err)
	}
	return nil
}

// Deactivate sets is_active = false for the given key.
func (s *VirtualKeyStore) Deactivate(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE virtual_keys SET is_active = false, updated_at = NOW() WHERE id = $1`
	tag, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deactivate virtual key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return auth.ErrKeyNotFound
	}
	return nil
}

// UpdateLastUsed sets last_used_at = NOW() for the given key.
func (s *VirtualKeyStore) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE virtual_keys SET last_used_at = NOW() WHERE id = $1`
	_, err := s.pool.Exec(ctx, q, id)
	return err
}

// UpdateHash rotates the key_hash and key_prefix for an existing virtual key.
func (s *VirtualKeyStore) UpdateHash(ctx context.Context, id uuid.UUID, keyHash, keyPrefix string) error {
	const q = `UPDATE virtual_keys SET key_hash = $1, key_prefix = $2, updated_at = NOW() WHERE id = $3`
	tag, err := s.pool.Exec(ctx, q, keyHash, keyPrefix, id)
	if err != nil {
		return fmt.Errorf("update key hash: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return auth.ErrKeyNotFound
	}
	return nil
}

// --- helpers ---

// scanner abstracts pgx.Row and pgx.Rows for the shared scan function.
type scanner interface {
	Scan(dest ...any) error
}

func scanVirtualKey(s scanner) (*auth.VirtualKey, error) {
	var (
		key           auth.VirtualKey
		id            pgtype.UUID
		userID        pgtype.UUID
		teamID        pgtype.UUID
		orgID         pgtype.UUID
		expiresAt     pgtype.Timestamptz
		budgetUSD     pgtype.Numeric
		rpmLimit      pgtype.Int4
		tpmLimit      pgtype.Int4
		allowedModels []string
		blockedModels []string
		lastUsedAt    pgtype.Timestamptz
	)

	err := s.Scan(
		&id, &key.KeyHash, &key.KeyPrefix, &key.Name,
		&userID, &teamID, &orgID,
		&expiresAt, &budgetUSD, &rpmLimit, &tpmLimit,
		&allowedModels, &blockedModels, &key.Metadata,
		&key.IsActive, &key.CreatedAt, &key.UpdatedAt, &lastUsedAt,
	)
	if err != nil {
		return nil, err
	}

	key.ID = uuid.UUID(id.Bytes)

	if userID.Valid {
		uid := uuid.UUID(userID.Bytes)
		key.UserID = &uid
	}
	if teamID.Valid {
		tid := uuid.UUID(teamID.Bytes)
		key.TeamID = &tid
	}
	if orgID.Valid {
		oid := uuid.UUID(orgID.Bytes)
		key.OrgID = &oid
	}
	if expiresAt.Valid {
		t := expiresAt.Time
		key.ExpiresAt = &t
	}
	if budgetUSD.Valid {
		f, _ := budgetUSD.Float64Value()
		if f.Valid {
			key.BudgetUSD = &f.Float64
		}
	}
	if rpmLimit.Valid {
		v := int(rpmLimit.Int32)
		key.RPMLimit = &v
	}
	if tpmLimit.Valid {
		v := int(tpmLimit.Int32)
		key.TPMLimit = &v
	}
	key.AllowedModels = allowedModels
	key.BlockedModels = blockedModels
	if lastUsedAt.Valid {
		t := lastUsedAt.Time
		key.LastUsedAt = &t
	}

	return &key, nil
}

// uuidPtrToParam converts an optional UUID to a value pgx can bind to a UUID column.
func uuidPtrToParam(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

// NewPool opens a pgxpool connection pool from the given DSN.
func NewPool(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	cfg.MaxConns = maxConns

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pgxpool: %w", err)
	}

	// Verify connectivity.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Register timestamptz → time.Time decoder (UTC).
	_ = time.UTC // ensure time package is imported
	return pool, nil
}
