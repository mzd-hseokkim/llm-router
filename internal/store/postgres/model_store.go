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

// ModelStore implements provider.ModelStore using PostgreSQL.
type ModelStore struct {
	pool *pgxpool.Pool
}

// NewModelStore creates a new store backed by the given connection pool.
func NewModelStore(pool *pgxpool.Pool) *ModelStore {
	return &ModelStore{pool: pool}
}

// ErrModelNotFound is returned when a model with the given ID does not exist.
var ErrModelNotFound = fmt.Errorf("model not found")

const modelSelectCols = `
	id, provider_id, model_id, model_name, display_name, is_enabled,
	input_per_million_tokens, output_per_million_tokens,
	context_window, max_output_tokens,
	supports_streaming, supports_tools, supports_vision,
	tags, sort_order, created_at, updated_at
`

// Create inserts a new model record.
func (s *ModelStore) Create(ctx context.Context, rec *provider.ModelRecord) error {
	const q = `
		INSERT INTO models (
			provider_id, model_id, model_name, display_name, is_enabled,
			input_per_million_tokens, output_per_million_tokens,
			context_window, max_output_tokens,
			supports_streaming, supports_tools, supports_vision,
			tags, sort_order
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, created_at, updated_at`

	row := s.pool.QueryRow(ctx, q,
		rec.ProviderID, rec.ModelID, rec.ModelName, rec.DisplayName, rec.IsEnabled,
		rec.InputPerMillionTokens, rec.OutputPerMillionTokens,
		rec.ContextWindow, rec.MaxOutputTokens,
		rec.SupportsStreaming, rec.SupportsTools, rec.SupportsVision,
		rec.Tags, rec.SortOrder,
	)

	var id pgtype.UUID
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &createdAt, &updatedAt); err != nil {
		return fmt.Errorf("create model: %w", err)
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

// GetByID retrieves a single model by its UUID.
func (s *ModelStore) GetByID(ctx context.Context, id uuid.UUID) (*provider.ModelRecord, error) {
	q := `SELECT` + modelSelectCols + `FROM models WHERE id = $1`
	row := s.pool.QueryRow(ctx, q, id)
	rec, err := scanModel(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrModelNotFound
	}
	return rec, err
}

// ListByProvider returns all models for the given provider ordered by sort_order, model_id.
func (s *ModelStore) ListByProvider(ctx context.Context, providerID uuid.UUID) ([]*provider.ModelRecord, error) {
	q := `SELECT` + modelSelectCols + `FROM models WHERE provider_id = $1 ORDER BY sort_order, model_id`
	return s.queryModels(ctx, q, providerID)
}

// ListEnabled returns all enabled models across all providers.
func (s *ModelStore) ListEnabled(ctx context.Context) ([]*provider.ModelRecord, error) {
	q := `SELECT` + modelSelectCols + `FROM models WHERE is_enabled = true ORDER BY sort_order, model_id`
	return s.queryModels(ctx, q)
}

// Update replaces mutable fields of an existing model.
func (s *ModelStore) Update(ctx context.Context, rec *provider.ModelRecord) error {
	const q = `
		UPDATE models SET
			model_name                = $1,
			display_name              = $2,
			is_enabled                = $3,
			input_per_million_tokens  = $4,
			output_per_million_tokens = $5,
			context_window            = $6,
			max_output_tokens         = $7,
			supports_streaming        = $8,
			supports_tools            = $9,
			supports_vision           = $10,
			tags                      = $11,
			sort_order                = $12,
			updated_at                = NOW()
		WHERE id = $13
		RETURNING updated_at`

	row := s.pool.QueryRow(ctx, q,
		rec.ModelName, rec.DisplayName, rec.IsEnabled,
		rec.InputPerMillionTokens, rec.OutputPerMillionTokens,
		rec.ContextWindow, rec.MaxOutputTokens,
		rec.SupportsStreaming, rec.SupportsTools, rec.SupportsVision,
		rec.Tags, rec.SortOrder, rec.ID,
	)
	var updatedAt pgtype.Timestamptz
	if err := row.Scan(&updatedAt); errors.Is(err, pgx.ErrNoRows) {
		return ErrModelNotFound
	} else if err != nil {
		return fmt.Errorf("update model: %w", err)
	}
	if updatedAt.Valid {
		rec.UpdatedAt = updatedAt.Time
	}
	return nil
}

// Delete permanently removes a model.
func (s *ModelStore) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM models WHERE id = $1`
	tag, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete model: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrModelNotFound
	}
	return nil
}

// --- helpers ---

func (s *ModelStore) queryModels(ctx context.Context, q string, args ...any) ([]*provider.ModelRecord, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query models: %w", err)
	}
	defer rows.Close()

	var recs []*provider.ModelRecord
	for rows.Next() {
		rec, err := scanModel(rows)
		if err != nil {
			return nil, fmt.Errorf("scan model: %w", err)
		}
		recs = append(recs, rec)
	}
	return recs, rows.Err()
}

func scanModel(s scanner) (*provider.ModelRecord, error) {
	var (
		rec                      provider.ModelRecord
		id                       pgtype.UUID
		providerID               pgtype.UUID
		displayName              pgtype.Text
		inputPerM, outputPerM    pgtype.Numeric
		contextWindow            pgtype.Int4
		maxOutputTokens          pgtype.Int4
		createdAt, updatedAt     pgtype.Timestamptz
	)

	err := s.Scan(
		&id, &providerID, &rec.ModelID, &rec.ModelName, &displayName, &rec.IsEnabled,
		&inputPerM, &outputPerM,
		&contextWindow, &maxOutputTokens,
		&rec.SupportsStreaming, &rec.SupportsTools, &rec.SupportsVision,
		&rec.Tags, &rec.SortOrder, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	rec.ID = uuid.UUID(id.Bytes)
	rec.ProviderID = uuid.UUID(providerID.Bytes)

	if displayName.Valid {
		rec.DisplayName = displayName.String
	}

	if inputPerM.Valid {
		f, _ := inputPerM.Float64Value()
		if f.Valid {
			rec.InputPerMillionTokens = f.Float64
		}
	}
	if outputPerM.Valid {
		f, _ := outputPerM.Float64Value()
		if f.Valid {
			rec.OutputPerMillionTokens = f.Float64
		}
	}

	if contextWindow.Valid {
		v := int(contextWindow.Int32)
		rec.ContextWindow = &v
	}
	if maxOutputTokens.Valid {
		v := int(maxOutputTokens.Int32)
		rec.MaxOutputTokens = &v
	}

	if createdAt.Valid {
		rec.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		rec.UpdatedAt = updatedAt.Time
	}
	return &rec, nil
}
