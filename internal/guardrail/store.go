package guardrail

import (
	"context"
	"time"
)

// PolicyRecord is a guardrail policy stored in the database.
type PolicyRecord struct {
	ID            string
	GuardrailType string
	IsEnabled     bool
	Action        string
	Engine        string
	ConfigJSON    []byte
	SortOrder     int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PolicyStore persists and retrieves guardrail policies.
type PolicyStore interface {
	List(ctx context.Context) ([]*PolicyRecord, error)
	GetByType(ctx context.Context, guardrailType string) (*PolicyRecord, error)
	Upsert(ctx context.Context, rec *PolicyRecord) error
	UpsertAll(ctx context.Context, recs []*PolicyRecord) error
}
