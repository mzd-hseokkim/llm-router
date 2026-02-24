package alerting

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/llm-router/gateway/internal/config"
)

// RuntimeConfig is the mutable alert configuration exposed via the admin API.
type RuntimeConfig struct {
	Enabled    bool                   `json:"enabled"`
	Channels   map[string]interface{} `json:"channels"`
	Conditions struct {
		BudgetThresholdPct  *float64 `json:"budget_threshold_pct,omitempty"`
		ErrorRateThreshold  *float64 `json:"error_rate_threshold,omitempty"`
		LatencyThresholdMs  *int     `json:"latency_threshold_ms,omitempty"`
	} `json:"conditions"`
}

// routeRule maps event patterns and severity to a set of notifiers.
type routeRule struct {
	events    []string // may contain wildcards like "security.*"
	severity  Severity
	notifiers []Notifier
}

// Router dispatches Alert Events to the appropriate channels.
type Router struct {
	rules  []routeRule
	dedup  *Deduplicator
	pool   *pgxpool.Pool
	logger *slog.Logger

	mu  sync.RWMutex
	cfg RuntimeConfig
}

// NewRouter builds a Router from the alerting config.
func NewRouter(
	cfg config.AlertingConfig,
	notifiers map[string]Notifier,
	dedup *Deduplicator,
	pool *pgxpool.Pool,
	logger *slog.Logger,
) *Router {
	rules := make([]routeRule, 0, len(cfg.Routing))
	for _, r := range cfg.Routing {
		var ns []Notifier
		for _, ch := range r.Channels {
			if n, ok := notifiers[ch]; ok {
				ns = append(ns, n)
			}
		}
		if len(ns) == 0 {
			continue
		}
		rules = append(rules, routeRule{
			events:    r.Events,
			severity:  Severity(r.Severity),
			notifiers: ns,
		})
	}

	ar := &Router{
		rules:  rules,
		dedup:  dedup,
		pool:   pool,
		logger: logger,
	}
	ar.cfg = runtimeConfigFromYAML(cfg)
	return ar
}

// runtimeConfigFromYAML converts the static YAML config to a RuntimeConfig.
func runtimeConfigFromYAML(cfg config.AlertingConfig) RuntimeConfig {
	channels := make(map[string]interface{})
	for _, ch := range cfg.Channels {
		switch ch.Type {
		case "slack":
			channels["slack"] = map[string]interface{}{
				"webhook_url": ch.WebhookURL,
				"enabled":     true,
			}
		case "email":
			channels["email"] = map[string]interface{}{
				"addresses": ch.To,
				"enabled":   true,
			}
		case "webhook":
			url := ch.URL
			if url == "" {
				url = ch.WebhookURL
			}
			channels["webhook"] = map[string]interface{}{
				"url":     url,
				"enabled": true,
			}
		}
	}
	return RuntimeConfig{Enabled: len(cfg.Channels) > 0, Channels: channels}
}

// GetConfig returns the current runtime alerting configuration.
func (r *Router) GetConfig() RuntimeConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cfg
}

// UpdateConfig replaces the runtime alerting configuration.
func (r *Router) UpdateConfig(cfg RuntimeConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = cfg
}

// Dispatch sends the event to all matching channels (async, deduplication applied).
func (r *Router) Dispatch(e *Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	if e.EntityID == "" {
		e.EntityID = "global"
	}

	go r.dispatch(e)
}

func (r *Router) dispatch(e *Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sent := false
	for _, rule := range r.rules {
		if !matchesRule(rule, e) {
			continue
		}

		for _, n := range rule.notifiers {
			if r.dedup != nil && !r.dedup.ShouldSend(ctx, e.EventType, e.EntityID+":"+n.Name()) {
				r.record(ctx, e, n.Name(), "deduplicated", nil)
				continue
			}

			err := n.Send(ctx, e)
			if err != nil {
				r.logger.Error("alerting: send failed",
					"channel", n.Name(),
					"event", e.EventType,
					"error", err)
				r.record(ctx, e, n.Name(), "failed", err)
			} else {
				r.logger.Info("alerting: sent",
					"channel", n.Name(),
					"event", e.EventType,
					"severity", e.Severity)
				r.record(ctx, e, n.Name(), "sent", nil)
				sent = true
			}
		}
	}

	if !sent && len(r.rules) > 0 {
		r.logger.Debug("alerting: no channel matched", "event", e.EventType)
	}
}

func matchesRule(rule routeRule, e *Event) bool {
	// Severity filter: if set, must match.
	if rule.severity != "" && rule.severity != e.Severity {
		return false
	}
	// Event filter: at least one pattern must match.
	for _, pattern := range rule.events {
		if matchPattern(pattern, e.EventType) {
			return true
		}
	}
	return false
}

// matchPattern supports a single trailing wildcard: "security.*" matches "security.pii".
func matchPattern(pattern, eventType string) bool {
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(eventType, prefix+".")
	}
	return pattern == eventType
}

// record persists an alert send attempt to the alert_history table.
func (r *Router) record(ctx context.Context, e *Event, channel, status string, sendErr error) {
	if r.pool == nil {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"event_type": e.EventType,
		"severity":   e.Severity,
		"title":      e.Title,
		"details":    e.Details,
	})
	var errMsg *string
	if sendErr != nil {
		s := sendErr.Error()
		errMsg = &s
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO alert_history (event_type, severity, channel, status, payload, error)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		e.EventType, string(e.Severity), channel, status, payload, errMsg,
	)
	if err != nil {
		r.logger.Error("alerting: failed to record history", "error", err)
	}
}
