package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AdminCredential holds admin login credentials.
type AdminCredential struct {
	ID              string
	Username        string
	PasswordHash    string
	PasswordChanged bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ErrAdminCredNotFound is returned when no matching credential is found.
var ErrAdminCredNotFound = errors.New("admin credential not found")

// AdminCredentialStore handles DB operations for admin credentials.
type AdminCredentialStore struct {
	pool *pgxpool.Pool
}

// NewAdminCredentialStore creates an AdminCredentialStore.
func NewAdminCredentialStore(pool *pgxpool.Pool) *AdminCredentialStore {
	return &AdminCredentialStore{pool: pool}
}

// GetByUsername retrieves admin credentials by username.
func (s *AdminCredentialStore) GetByUsername(ctx context.Context, username string) (*AdminCredential, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, password_changed, created_at, updated_at
		 FROM admin_credentials WHERE username = $1`, username)

	var c AdminCredential
	err := row.Scan(&c.ID, &c.Username, &c.PasswordHash, &c.PasswordChanged, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAdminCredNotFound
		}
		return nil, fmt.Errorf("admin_credential_store.GetByUsername: %w", err)
	}
	return &c, nil
}

// UpsertDefault inserts a default admin account if none exists.
func (s *AdminCredentialStore) UpsertDefault(ctx context.Context, username, hash string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO admin_credentials (username, password_hash)
		 VALUES ($1, $2)
		 ON CONFLICT (username) DO NOTHING`, username, hash)
	if err != nil {
		return fmt.Errorf("admin_credential_store.UpsertDefault: %w", err)
	}
	return nil
}

// UpdatePassword sets a new password hash and marks the password as changed.
func (s *AdminCredentialStore) UpdatePassword(ctx context.Context, username, newHash string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE admin_credentials
		 SET password_hash = $1, password_changed = true, updated_at = NOW()
		 WHERE username = $2`, newHash, username)
	if err != nil {
		return fmt.Errorf("admin_credential_store.UpdatePassword: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAdminCredNotFound
	}
	return nil
}
