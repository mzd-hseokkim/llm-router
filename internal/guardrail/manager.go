package guardrail

import (
	"log/slog"
	"sync/atomic"
)

// Manager holds the active Pipeline and supports atomic hot-reload.
// Reads are lock-free; writes are goroutine-safe via atomic.Pointer.
type Manager struct {
	pipeline atomic.Pointer[Pipeline]
	logger   *slog.Logger
}

// NewManager creates a new Manager with no active pipeline.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{logger: logger}
}

// Pipeline returns the currently active pipeline. May be nil if none is set.
func (m *Manager) Pipeline() *Pipeline {
	return m.pipeline.Load()
}

// SetPipeline atomically replaces the active pipeline.
// Safe to call from any goroutine while requests are in flight.
func (m *Manager) SetPipeline(p *Pipeline) {
	m.pipeline.Store(p)
	if p == nil {
		m.logger.Info("guardrails disabled")
	} else {
		m.logger.Info("guardrails reloaded")
	}
}
