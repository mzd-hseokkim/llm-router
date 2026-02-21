package handler

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

// AdminAuditHandler serves audit log query and CSV export endpoints.
type AdminAuditHandler struct {
	store *pgstore.AuditStore
}

// NewAdminAuditHandler creates an AdminAuditHandler.
func NewAdminAuditHandler(store *pgstore.AuditStore) *AdminAuditHandler {
	return &AdminAuditHandler{store: store}
}

// List handles GET /admin/audit-logs
func (h *AdminAuditHandler) List(w http.ResponseWriter, r *http.Request) {
	f := parseAuditFilter(r)

	if r.URL.Query().Get("format") == "csv" {
		h.exportCSV(w, r, f)
		return
	}

	events, total, err := h.store.List(r.Context(), f)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"total":  total,
		"page":   f.Page,
		"limit":  f.Limit,
		"events": events,
	})
}

// SecurityEvents handles GET /admin/audit-logs/security-events
func (h *AdminAuditHandler) SecurityEvents(w http.ResponseWriter, r *http.Request) {
	f := parseAuditFilter(r)
	f.SecurityOnly = true

	events, total, err := h.store.List(r.Context(), f)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"total":  total,
		"page":   f.Page,
		"limit":  f.Limit,
		"events": events,
	})
}

func (h *AdminAuditHandler) exportCSV(w http.ResponseWriter, r *http.Request, f pgstore.AuditFilter) {
	f.Limit = 5000
	events, _, err := h.store.List(context.Background(), f)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="audit_logs.csv"`)

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"timestamp", "event_type", "action", "actor_type", "actor_email",
		"ip_address", "resource_type", "resource_name", "request_id",
	})

	for _, e := range events {
		_ = cw.Write([]string{
			e.Timestamp.Format(time.RFC3339),
			e.EventType,
			e.Action,
			e.ActorType,
			e.ActorEmail,
			e.IPAddress,
			e.ResourceType,
			e.ResourceName,
			e.RequestID,
		})
	}
	cw.Flush()
}

func parseAuditFilter(r *http.Request) pgstore.AuditFilter {
	q := r.URL.Query()
	f := pgstore.AuditFilter{
		ActorID:    q.Get("actor_id"),
		EventType:  q.Get("event_type"),
		ResourceID: q.Get("resource_id"),
	}

	if s := q.Get("from"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.From = t
		} else if t, err := time.Parse("2006-01-02", s); err == nil {
			f.From = t
		}
	}
	if s := q.Get("to"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.To = t
		} else if t, err := time.Parse("2006-01-02", s); err == nil {
			f.To = t.Add(24*time.Hour - time.Second) // end of day
		}
	}

	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	f.Page, _ = strconv.Atoi(q.Get("page"))

	return f
}
