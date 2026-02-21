package telemetry

import (
	"sync"
)

// LatencyTracker tracks per-provider EWMA (Exponential Weighted Moving Average) latency.
// It is safe for concurrent use.
type LatencyTracker struct {
	mu    sync.RWMutex
	ewma  map[string]float64
	alpha float64 // smoothing factor: 0 < alpha ≤ 1
}

// NewLatencyTracker returns a LatencyTracker with the given smoothing factor.
// alpha = 0.1 gives a slowly adapting average (more stable).
// alpha = 0.5 gives a faster-adapting average (more responsive).
func NewLatencyTracker(alpha float64) *LatencyTracker {
	if alpha <= 0 || alpha > 1 {
		alpha = 0.1
	}
	return &LatencyTracker{
		ewma:  make(map[string]float64),
		alpha: alpha,
	}
}

// Record updates the EWMA for provider with a new latency sample in milliseconds.
func (t *LatencyTracker) Record(provider string, latencyMs float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	prev, ok := t.ewma[provider]
	if !ok {
		t.ewma[provider] = latencyMs
		return
	}
	t.ewma[provider] = t.alpha*latencyMs + (1-t.alpha)*prev
}

// Get returns the current EWMA latency for provider in milliseconds.
// Returns 0 if no samples have been recorded.
func (t *LatencyTracker) Get(provider string) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.ewma[provider]
}

// All returns a snapshot of all tracked EWMA values keyed by provider name.
func (t *LatencyTracker) All() map[string]float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make(map[string]float64, len(t.ewma))
	for k, v := range t.ewma {
		out[k] = v
	}
	return out
}
