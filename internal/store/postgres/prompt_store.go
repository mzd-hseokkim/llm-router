package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PromptRow represents a row in the prompts table.
type PromptRow struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	TeamID      *string   `json:"team_id,omitempty"`
	Visibility  string    `json:"visibility"`
	CreatedBy   *string   `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// PromptVersionRow represents a row in the prompt_versions table.
type PromptVersionRow struct {
	ID         string          `json:"id"`
	PromptID   string          `json:"prompt_id"`
	Version    string          `json:"version"`
	Status     string          `json:"status"`
	Template   string          `json:"template"`
	Variables  json.RawMessage `json:"variables"`
	Parameters json.RawMessage `json:"parameters"`
	Model      string          `json:"model"`
	Changelog  string          `json:"changelog"`
	CreatedBy  *string         `json:"created_by,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// PromptStore handles prompt and prompt version persistence.
type PromptStore struct {
	pool *pgxpool.Pool
}

// NewPromptStore returns a PromptStore backed by pool.
func NewPromptStore(pool *pgxpool.Pool) *PromptStore {
	return &PromptStore{pool: pool}
}

// CreatePrompt inserts a new prompt and returns it.
func (s *PromptStore) CreatePrompt(ctx context.Context, slug, name, description, visibility string, teamID, createdBy *string) (*PromptRow, error) {
	row := &PromptRow{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO prompts (slug, name, description, visibility, team_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, slug, name, COALESCE(description,''), visibility,
		          team_id::TEXT, created_by::TEXT, created_at`,
		slug, name, description, visibility, teamID, createdBy,
	).Scan(&row.ID, &row.Slug, &row.Name, &row.Description, &row.Visibility,
		&row.TeamID, &row.CreatedBy, &row.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("prompt_store create: %w", err)
	}
	return row, nil
}

// GetPromptBySlug returns the prompt with the given slug.
func (s *PromptStore) GetPromptBySlug(ctx context.Context, slug string) (*PromptRow, error) {
	row := &PromptRow{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, slug, name, COALESCE(description,''), visibility,
		       team_id::TEXT, created_by::TEXT, created_at
		FROM prompts WHERE slug = $1`, slug,
	).Scan(&row.ID, &row.Slug, &row.Name, &row.Description, &row.Visibility,
		&row.TeamID, &row.CreatedBy, &row.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("prompt_store get: %w", err)
	}
	return row, nil
}

// ListPrompts returns all prompts (optionally filtered by teamID).
func (s *PromptStore) ListPrompts(ctx context.Context, teamID *string) ([]*PromptRow, error) {
	query := `SELECT id, slug, name, COALESCE(description,''), visibility,
		       team_id::TEXT, created_by::TEXT, created_at FROM prompts`
	args := []any{}
	if teamID != nil {
		query += " WHERE team_id = $1"
		args = append(args, *teamID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("prompt_store list: %w", err)
	}
	defer rows.Close()

	var result []*PromptRow
	for rows.Next() {
		r := &PromptRow{}
		if err := rows.Scan(&r.ID, &r.Slug, &r.Name, &r.Description, &r.Visibility,
			&r.TeamID, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// CreateVersion inserts a new version for the given prompt.
func (s *PromptStore) CreateVersion(ctx context.Context, promptID, version, status, template, model, changelog string, variables, parameters json.RawMessage, createdBy *string) (*PromptVersionRow, error) {
	row := &PromptVersionRow{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO prompt_versions (prompt_id, version, status, template, variables, parameters, model, changelog, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, prompt_id, version, status, template, variables, parameters,
		          COALESCE(model,''), COALESCE(changelog,''), created_by::TEXT, created_at`,
		promptID, version, status, template, variables, parameters, model, changelog, createdBy,
	).Scan(&row.ID, &row.PromptID, &row.Version, &row.Status, &row.Template,
		&row.Variables, &row.Parameters, &row.Model, &row.Changelog, &row.CreatedBy, &row.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("prompt_store create_version: %w", err)
	}
	return row, nil
}

// GetActiveVersion returns the version with status='active' for the given prompt.
func (s *PromptStore) GetActiveVersion(ctx context.Context, promptID string) (*PromptVersionRow, error) {
	row := &PromptVersionRow{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, prompt_id, version, status, template, variables, parameters,
		       COALESCE(model,''), COALESCE(changelog,''), created_by::TEXT, created_at
		FROM prompt_versions WHERE prompt_id = $1 AND status = 'active'
		ORDER BY created_at DESC LIMIT 1`, promptID,
	).Scan(&row.ID, &row.PromptID, &row.Version, &row.Status, &row.Template,
		&row.Variables, &row.Parameters, &row.Model, &row.Changelog, &row.CreatedBy, &row.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("prompt_store get_active: %w", err)
	}
	return row, nil
}

// GetVersion returns a specific version by slug+version string.
func (s *PromptStore) GetVersion(ctx context.Context, promptID, version string) (*PromptVersionRow, error) {
	row := &PromptVersionRow{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, prompt_id, version, status, template, variables, parameters,
		       COALESCE(model,''), COALESCE(changelog,''), created_by::TEXT, created_at
		FROM prompt_versions WHERE prompt_id = $1 AND version = $2`, promptID, version,
	).Scan(&row.ID, &row.PromptID, &row.Version, &row.Status, &row.Template,
		&row.Variables, &row.Parameters, &row.Model, &row.Changelog, &row.CreatedBy, &row.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("prompt_store get_version: %w", err)
	}
	return row, nil
}

// ListVersions returns all versions for the given prompt, newest first.
func (s *PromptStore) ListVersions(ctx context.Context, promptID string) ([]*PromptVersionRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, prompt_id, version, status, template, variables, parameters,
		       COALESCE(model,''), COALESCE(changelog,''), created_by::TEXT, created_at
		FROM prompt_versions WHERE prompt_id = $1
		ORDER BY created_at DESC`, promptID)
	if err != nil {
		return nil, fmt.Errorf("prompt_store list_versions: %w", err)
	}
	defer rows.Close()

	var result []*PromptVersionRow
	for rows.Next() {
		r := &PromptVersionRow{}
		if err := rows.Scan(&r.ID, &r.PromptID, &r.Version, &r.Status, &r.Template,
			&r.Variables, &r.Parameters, &r.Model, &r.Changelog, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// SetVersionStatus changes the status of a specific version.
// When activating, it also deprecates the previously active version.
func (s *PromptStore) SetVersionStatus(ctx context.Context, promptID, version, status string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("prompt_store set_status begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if status == "active" {
		// Deprecate any currently active version first.
		_, err = tx.Exec(ctx,
			`UPDATE prompt_versions SET status = 'deprecated'
			 WHERE prompt_id = $1 AND status = 'active' AND version != $2`,
			promptID, version)
		if err != nil {
			return fmt.Errorf("prompt_store deprecate: %w", err)
		}
	}

	_, err = tx.Exec(ctx,
		`UPDATE prompt_versions SET status = $1 WHERE prompt_id = $2 AND version = $3`,
		status, promptID, version)
	if err != nil {
		return fmt.Errorf("prompt_store set_status: %w", err)
	}

	return tx.Commit(ctx)
}
