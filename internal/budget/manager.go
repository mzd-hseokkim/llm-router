package budget

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// ErrBudgetExceeded is returned when a hard budget limit is reached.
type ErrBudgetExceeded struct {
	Current float64
	Limit   float64
	Period  string
}

func (e ErrBudgetExceeded) Error() string {
	return fmt.Sprintf("budget exceeded: current $%.4f >= hard limit $%.4f (%s)", e.Current, e.Limit, e.Period)
}

// Budget represents a stored budget record.
type Budget struct {
	ID           uuid.UUID
	EntityType   string
	EntityID     uuid.UUID
	Period       string
	SoftLimitUSD *float64
	HardLimitUSD *float64
	CurrentSpend float64
	PeriodStart  time.Time
	PeriodEnd    time.Time
}

// Store is the persistence interface for budgets.
type Store interface {
	Get(ctx context.Context, entityType string, entityID uuid.UUID, period string) (*Budget, error)
	List(ctx context.Context, entityType string, entityID uuid.UUID) ([]*Budget, error)
	Upsert(ctx context.Context, b *Budget) error
	AddSpend(ctx context.Context, entityType string, entityID uuid.UUID, period string, amountUSD float64) error
	ResetExpired(ctx context.Context, now time.Time) (int, error)
}

// Cache is the Redis interface for fast spend tracking.
type Cache interface {
	// IncrSpend atomically adds amountUSD and returns the new total.
	IncrSpend(ctx context.Context, entityType string, entityID uuid.UUID, period string, amountUSD float64) (float64, error)
	// GetSpend returns the current spend and whether the key exists.
	GetSpend(ctx context.Context, entityType string, entityID uuid.UUID, period string) (float64, bool, error)
	// SetSpend initialises the cache from a DB value.
	SetSpend(ctx context.Context, entityType string, entityID uuid.UUID, period string, amountUSD float64) error
	// DeleteSpend removes a spend key (called on period reset).
	DeleteSpend(ctx context.Context, entityType string, entityID uuid.UUID, period string) error
}

// Manager checks and records budget spend for LLM requests.
type Manager struct {
	store  Store
	cache  Cache
	logger *slog.Logger
}

// NewManager creates a Manager.
func NewManager(store Store, cache Cache, logger *slog.Logger) *Manager {
	return &Manager{store: store, cache: cache, logger: logger}
}

// CheckBudget verifies that the entity has not exceeded its hard budget for the
// active period. Returns ErrBudgetExceeded if the limit is reached, nil if
// no budget is configured (= unlimited) or the budget has headroom.
func (m *Manager) CheckBudget(ctx context.Context, entityType string, entityID uuid.UUID) error {
	budgets, err := m.store.List(ctx, entityType, entityID)
	if err != nil {
		// On store error, allow the request (fail open).
		m.logger.Warn("budget store list error; allowing request", "error", err,
			"entity_type", entityType, "entity_id", entityID)
		return nil
	}

	for _, b := range budgets {
		if b.HardLimitUSD == nil {
			continue
		}
		spend, err := m.currentSpend(ctx, b)
		if err != nil {
			m.logger.Warn("budget cache error; using DB value", "error", err)
			spend = b.CurrentSpend
		}
		if spend >= *b.HardLimitUSD {
			return ErrBudgetExceeded{
				Current: spend,
				Limit:   *b.HardLimitUSD,
				Period:  b.Period,
			}
		}
		if b.SoftLimitUSD != nil && spend >= *b.SoftLimitUSD {
			m.logger.Warn("soft budget limit reached",
				"entity_type", entityType, "entity_id", entityID,
				"period", b.Period, "spend", spend, "soft_limit", *b.SoftLimitUSD)
		}
	}
	return nil
}

// RecordSpend records costUSD of spend for entityID. Errors are logged but not
// returned — the request has already been completed.
func (m *Manager) RecordSpend(ctx context.Context, entityType string, entityID uuid.UUID, costUSD float64) {
	if costUSD == 0 {
		return
	}
	budgets, err := m.store.List(ctx, entityType, entityID)
	if err != nil {
		m.logger.Error("budget record spend: list error", "error", err)
		return
	}
	for _, b := range budgets {
		if _, err := m.cache.IncrSpend(ctx, entityType, entityID, b.Period, costUSD); err != nil {
			m.logger.Error("budget cache incr error", "error", err, "period", b.Period)
		}
		// DB is synced by the background scheduler.
	}
}

// currentSpend returns the entity's spend from cache, falling back to DB.
func (m *Manager) currentSpend(ctx context.Context, b *Budget) (float64, error) {
	spend, ok, err := m.cache.GetSpend(ctx, b.EntityType, b.EntityID, b.Period)
	if err != nil {
		return 0, err
	}
	if ok {
		return spend, nil
	}
	// Cache miss: seed from DB.
	if err := m.cache.SetSpend(ctx, b.EntityType, b.EntityID, b.Period, b.CurrentSpend); err != nil {
		return b.CurrentSpend, err
	}
	return b.CurrentSpend, nil
}

// SyncDB flushes Redis spend counters back to the DB. Called by the scheduler.
func (m *Manager) SyncDB(ctx context.Context) {
	// List all active budgets and sync cache → DB.
	// For Phase 2, each IncrSpend call also calls store.AddSpend (see redis_cache).
	// This method is a no-op placeholder for a future batched sync.
}

// IsBudgetExceeded returns true if err is an ErrBudgetExceeded.
func IsBudgetExceeded(err error) (ErrBudgetExceeded, bool) {
	var e ErrBudgetExceeded
	if errors.As(err, &e) {
		return e, true
	}
	return ErrBudgetExceeded{}, false
}
