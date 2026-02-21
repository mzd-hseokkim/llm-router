package provider

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ProviderRecord maps to the providers table.
type ProviderRecord struct {
	ID          uuid.UUID
	Name        string
	AdapterType string
	DisplayName string
	BaseURL     string
	IsEnabled   bool
	ConfigJSON  []byte // raw JSONB
	SortOrder   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ModelRecord maps to the models table.
type ModelRecord struct {
	ID                      uuid.UUID
	ProviderID              uuid.UUID
	ModelID                 string
	ModelName               string
	DisplayName             string
	IsEnabled               bool
	InputPerMillionTokens   float64
	OutputPerMillionTokens  float64
	ContextWindow           *int
	MaxOutputTokens         *int
	SupportsStreaming        bool
	SupportsTools           bool
	SupportsVision          bool
	Tags                    []string
	SortOrder               int
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// ProviderStore persists provider records.
type ProviderStore interface {
	Create(ctx context.Context, rec *ProviderRecord) error
	GetByID(ctx context.Context, id uuid.UUID) (*ProviderRecord, error)
	GetByName(ctx context.Context, name string) (*ProviderRecord, error)
	List(ctx context.Context) ([]*ProviderRecord, error)
	Update(ctx context.Context, rec *ProviderRecord) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// ModelStore persists model records.
type ModelStore interface {
	Create(ctx context.Context, rec *ModelRecord) error
	GetByID(ctx context.Context, id uuid.UUID) (*ModelRecord, error)
	ListByProvider(ctx context.Context, providerID uuid.UUID) ([]*ModelRecord, error)
	ListEnabled(ctx context.Context) ([]*ModelRecord, error)
	Update(ctx context.Context, rec *ModelRecord) error
	Delete(ctx context.Context, id uuid.UUID) error
}
