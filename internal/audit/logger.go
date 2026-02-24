package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Event represents a single auditable action.
type Event struct {
	ID           *uuid.UUID     `json:"id"`
	EventType    string         `json:"event_type"`
	Action       string         `json:"action"`
	ActorType    string         `json:"actor_type"`
	ActorID      *uuid.UUID     `json:"actor_id"`
	ActorEmail   string         `json:"actor_email"`
	IPAddress    string         `json:"ip_address"`
	UserAgent    string         `json:"user_agent"`
	ResourceType string         `json:"resource_type"`
	ResourceID   *uuid.UUID     `json:"resource_id"`
	ResourceName string         `json:"resource_name"`
	Changes      map[string]any `json:"changes"` // {"before": {...}, "after": {...}, "changed_fields": [...]}
	Metadata     map[string]any `json:"metadata"`
	RequestID    string         `json:"request_id"`
	OrgID        *uuid.UUID     `json:"org_id"`
	TeamID       *uuid.UUID     `json:"team_id"`
	Timestamp    time.Time      `json:"timestamp"`
}

// Store persists audit events.
type Store interface {
	Insert(ctx context.Context, e *Event) error
}

// Logger is the public audit-log API.
type Logger struct {
	store  Store
	logger *slog.Logger
	ch     chan *Event
	done   chan struct{}
}

// New creates an audit Logger that buffers up to 1000 events and writes them asynchronously.
func New(store Store, logger *slog.Logger) *Logger {
	l := &Logger{
		store:  store,
		logger: logger,
		ch:     make(chan *Event, 1000),
		done:   make(chan struct{}),
	}
	go l.run()
	return l
}

// Record enqueues an event for async persistence. Never blocks the caller.
func (l *Logger) Record(e *Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	select {
	case l.ch <- e:
	default:
		l.logger.Warn("audit: event queue full; dropping event", "event_type", e.EventType)
	}
}

// Close drains the queue and waits for the background worker to finish.
func (l *Logger) Close() {
	close(l.ch)
	<-l.done
}

func (l *Logger) run() {
	defer close(l.done)
	for e := range l.ch {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := l.store.Insert(ctx, e); err != nil {
			b, _ := json.Marshal(e)
			l.logger.Error("audit: failed to persist event", "error", err, "event", string(b))
		}
		cancel()
	}
}

// BuildChanges is a convenience helper that produces a Changes map from before/after snapshots.
// Sensitive fields listed in redactFields are replaced with "[REDACTED]".
func BuildChanges(before, after map[string]any, redactFields ...string) map[string]any {
	redact := make(map[string]bool, len(redactFields))
	for _, f := range redactFields {
		redact[f] = true
	}

	mask := func(m map[string]any) map[string]any {
		out := make(map[string]any, len(m))
		for k, v := range m {
			if redact[k] {
				out[k] = "[REDACTED]"
			} else {
				out[k] = v
			}
		}
		return out
	}

	var changed []string
	for k := range after {
		bv, _ := json.Marshal(before[k])
		av, _ := json.Marshal(after[k])
		if string(bv) != string(av) {
			if !redact[k] {
				changed = append(changed, k)
			}
		}
	}

	return map[string]any{
		"before":         mask(before),
		"after":          mask(after),
		"changed_fields": changed,
	}
}
