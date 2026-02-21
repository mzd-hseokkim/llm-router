package health

import (
	"sync"
	"time"
)

const slidingWindow = time.Minute

// ProviderTracker tracks recent request outcomes per provider using a 1-minute sliding window.
// It is safe for concurrent use.
type ProviderTracker struct {
	mu      sync.Mutex
	entries map[string][]providerEntry
}

type providerEntry struct {
	ts      time.Time
	success bool
}

// NewProviderTracker creates a ProviderTracker.
func NewProviderTracker() *ProviderTracker {
	return &ProviderTracker{entries: make(map[string][]providerEntry)}
}

// Record records a request outcome for the given provider.
// success should be true when the HTTP response status < 500.
func (t *ProviderTracker) Record(providerName string, success bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries[providerName] = append(t.entries[providerName], providerEntry{time.Now(), success})
	t.pruneUnlocked(providerName)
}

// ErrorRate returns the fraction of failed requests [0, 1] in the last minute.
// Returns 0 if there are no recent requests.
func (t *ProviderTracker) ErrorRate(providerName string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneUnlocked(providerName)
	entries := t.entries[providerName]
	if len(entries) == 0 {
		return 0
	}
	var errs int
	for _, e := range entries {
		if !e.success {
			errs++
		}
	}
	return float64(errs) / float64(len(entries))
}

// Status returns "ok", "degraded", or "unhealthy" based on the 1-minute error rate.
//
//	error_rate >= 0.50 → "unhealthy"
//	error_rate >= 0.10 → "degraded"
//	otherwise         → "ok"
func (t *ProviderTracker) Status(providerName string) string {
	rate := t.ErrorRate(providerName)
	switch {
	case rate >= 0.5:
		return "unhealthy"
	case rate >= 0.1:
		return "degraded"
	default:
		return "ok"
	}
}

// pruneUnlocked removes entries older than the sliding window.
// Must be called with t.mu held.
func (t *ProviderTracker) pruneUnlocked(providerName string) {
	cutoff := time.Now().Add(-slidingWindow)
	entries := t.entries[providerName]
	i := 0
	for i < len(entries) && entries[i].ts.Before(cutoff) {
		i++
	}
	t.entries[providerName] = entries[i:]
}
