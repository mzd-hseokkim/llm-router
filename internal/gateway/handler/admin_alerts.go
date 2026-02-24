package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/llm-router/gateway/internal/alerting"
)

// AdminAlertsHandler handles /admin/alerts/* endpoints.
type AdminAlertsHandler struct {
	router *alerting.Router
	pool   *pgxpool.Pool
}

// NewAdminAlertsHandler creates an AdminAlertsHandler.
func NewAdminAlertsHandler(router *alerting.Router, pool *pgxpool.Pool) *AdminAlertsHandler {
	return &AdminAlertsHandler{router: router, pool: pool}
}

// Test handles POST /admin/alerts/test — sends a test alert to one or all channels.
func (h *AdminAlertsHandler) Test(w http.ResponseWriter, r *http.Request) {
	channel := r.URL.Query().Get("channel")
	e := &alerting.Event{
		EventType: "system.test",
		Severity:  alerting.SeverityInfo,
		Title:     "Test Alert",
		Details: map[string]any{
			"channel":   channel,
			"requested": "manual test via admin API",
		},
		Timestamp: time.Now().UTC(),
		EntityID:  "test-" + strconv.FormatInt(time.Now().UnixMilli(), 36),
	}

	h.router.Dispatch(e)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "dispatched"}) //nolint:errcheck
}

// History handles GET /admin/alerts/history — returns recent alert sends.
func (h *AdminAlertsHandler) History(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	rows, err := h.pool.Query(context.Background(), `
		SELECT id, event_type, severity, channel, status, payload, error, sent_at
		FROM alert_history
		ORDER BY sent_at DESC
		LIMIT $1`, limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type record struct {
		ID        string `json:"id"`
		EventType string `json:"event_type"`
		Severity  string `json:"severity"`
		Channel   string `json:"channel"`
		Status    string `json:"status"`
		Payload   any    `json:"payload"`
		Error     *string `json:"error,omitempty"`
		SentAt    string `json:"sent_at"`
	}

	results := make([]record, 0)
	for rows.Next() {
		var rec record
		var payloadRaw []byte
		var sentAt time.Time
		if err := rows.Scan(&rec.ID, &rec.EventType, &rec.Severity, &rec.Channel,
			&rec.Status, &payloadRaw, &rec.Error, &sentAt); err != nil {
			continue
		}
		rec.SentAt = sentAt.Format(time.RFC3339)
		_ = json.Unmarshal(payloadRaw, &rec.Payload)
		results = append(results, rec)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"history": results}) //nolint:errcheck
}

// GetConfig handles GET /admin/alerts/config.
func (h *AdminAlertsHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	if h.router == nil {
		// No alerting configured — return empty defaults.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(alerting.RuntimeConfig{ //nolint:errcheck
			Enabled:  false,
			Channels: map[string]interface{}{},
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.router.GetConfig()) //nolint:errcheck
}

// UpdateConfig handles PUT /admin/alerts/config.
func (h *AdminAlertsHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	if h.router == nil {
		http.Error(w, "alerting not configured", http.StatusServiceUnavailable)
		return
	}
	var cfg alerting.RuntimeConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	h.router.UpdateConfig(cfg)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg) //nolint:errcheck
}
