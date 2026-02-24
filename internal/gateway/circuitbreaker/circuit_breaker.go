// Package circuitbreaker implements a per-provider circuit breaker.
// State transitions: Closed → Open (on N consecutive failures) → HalfOpen (after timeout) → Closed (on M successes).
package circuitbreaker

import (
	"sync"
	"time"
)

// State represents the circuit breaker state for a provider.
type State int

const (
	StateClosed   State = iota // normal operation; requests pass through
	StateOpen                  // circuit tripped; requests are blocked
	StateHalfOpen              // testing recovery; one probe request allowed
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// Config holds circuit breaker thresholds.
type Config struct {
	FailureThreshold int           // consecutive failures before opening (default 5)
	SuccessThreshold int           // consecutive successes in HalfOpen to close (default 2)
	OpenTimeout      time.Duration // time in Open state before probing (default 60s)
}

// DefaultConfig returns production-ready defaults.
func DefaultConfig() Config {
	return Config{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		OpenTimeout:      60 * time.Second,
	}
}

type entry struct {
	state         State
	failures      int
	successes     int
	lastFailureAt time.Time
}

// CircuitBreaker tracks per-provider circuit state.
// It is safe for concurrent use.
type CircuitBreaker struct {
	mu      sync.Mutex
	entries map[string]*entry
	cfg     Config
}

// New returns a CircuitBreaker with the given configuration.
func New(cfg Config) *CircuitBreaker {
	return &CircuitBreaker{
		entries: make(map[string]*entry),
		cfg:     cfg,
	}
}

func (cb *CircuitBreaker) get(provider string) *entry {
	e, ok := cb.entries[provider]
	if !ok {
		e = &entry{state: StateClosed}
		cb.entries[provider] = e
	}
	return e
}

// IsOpen returns true if requests to provider should be blocked.
// A transition from Open→HalfOpen happens when OpenTimeout elapses.
func (cb *CircuitBreaker) IsOpen(provider string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	e := cb.get(provider)
	if e.state == StateOpen {
		if time.Since(e.lastFailureAt) >= cb.cfg.OpenTimeout {
			e.state = StateHalfOpen
			e.successes = 0
			return false // allow probe request
		}
		return true
	}
	return false
}

// RecordSuccess records a successful request for provider.
func (cb *CircuitBreaker) RecordSuccess(provider string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	e := cb.get(provider)
	switch e.state {
	case StateHalfOpen:
		e.successes++
		if e.successes >= cb.cfg.SuccessThreshold {
			e.state = StateClosed
			e.failures = 0
			e.successes = 0
		}
	case StateClosed:
		e.failures = 0
	}
}

// RecordFailure records a failed request for provider.
func (cb *CircuitBreaker) RecordFailure(provider string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	e := cb.get(provider)
	e.failures++
	e.lastFailureAt = time.Now()

	switch e.state {
	case StateClosed:
		if e.failures >= cb.cfg.FailureThreshold {
			e.state = StateOpen
		}
	case StateHalfOpen:
		// Any failure in HalfOpen re-opens the circuit immediately.
		e.state = StateOpen
		e.failures = 0
	}
}

// Reset manually closes the circuit for provider (e.g. via admin API).
func (cb *CircuitBreaker) Reset(provider string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.entries[provider] = &entry{state: StateClosed}
}

// AllStatus returns the current state string for every tracked provider.
func (cb *CircuitBreaker) AllStatus() map[string]string {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	out := make(map[string]string, len(cb.entries))
	for p, e := range cb.entries {
		out[p] = e.state.String()
	}
	return out
}

// ProviderStatusDetail holds detailed status for a single provider.
type ProviderStatusDetail struct {
	Provider     string     `json:"provider"`
	State        string     `json:"state"`
	FailureCount int        `json:"failure_count"`
	LastFailure  *time.Time `json:"last_failure,omitempty"`
	ResetTime    *time.Time `json:"reset_time,omitempty"`
}

// AllStatusDetailed returns per-provider status with failure counts and timestamps.
func (cb *CircuitBreaker) AllStatusDetailed() []ProviderStatusDetail {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	out := make([]ProviderStatusDetail, 0, len(cb.entries))
	for p, e := range cb.entries {
		d := ProviderStatusDetail{
			Provider:     p,
			State:        e.state.String(),
			FailureCount: e.failures,
		}
		if !e.lastFailureAt.IsZero() {
			t := e.lastFailureAt
			d.LastFailure = &t
			if e.state == StateOpen {
				rt := e.lastFailureAt.Add(cb.cfg.OpenTimeout)
				d.ResetTime = &rt
			}
		}
		out = append(out, d)
	}
	return out
}

// ProviderStatus returns the state string for a specific provider.
// Returns "closed" for providers with no recorded events.
func (cb *CircuitBreaker) ProviderStatus(provider string) string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.get(provider).state.String()
}
