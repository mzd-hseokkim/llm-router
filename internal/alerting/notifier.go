package alerting

import (
	"context"
	"time"
)

// Severity levels for alerts.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// Event represents an alert event to be dispatched.
type Event struct {
	EventType string
	Severity  Severity
	Title     string
	Details   map[string]any
	Timestamp time.Time
	EntityID  string // used for deduplication key
}

// Notifier sends alerts to a specific channel.
type Notifier interface {
	// Name returns the channel name (for logging and history).
	Name() string
	// Send transmits the event. Returns an error on failure.
	Send(ctx context.Context, e *Event) error
}
