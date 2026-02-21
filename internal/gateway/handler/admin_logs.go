package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/store/postgres"
	"github.com/llm-router/gateway/internal/telemetry"
)

// logQuerier is the minimal interface needed by AdminLogsHandler.
type logQuerier interface {
	List(ctx context.Context, f postgres.LogFilter) ([]*telemetry.LogEntry, error)
}

// AdminLogsHandler handles GET /admin/logs.
type AdminLogsHandler struct {
	store logQuerier
}

// NewAdminLogsHandler returns an AdminLogsHandler backed by the given store.
func NewAdminLogsHandler(store logQuerier) *AdminLogsHandler {
	return &AdminLogsHandler{store: store}
}

// List handles GET /admin/logs with optional query parameters:
//
//	key_id  — filter by virtual key UUID
//	from    — start time (RFC3339), default: 7 days ago
//	to      — end time   (RFC3339), default: now
//	limit   — max entries (1–1000), default: 100
//	offset  — pagination offset, default: 0
func (h *AdminLogsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var f postgres.LogFilter

	if s := q.Get("key_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid key_id: "+err.Error(), "invalid_request_error", "")
			return
		}
		f.VirtualKeyID = &id
	}

	if s := q.Get("from"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from: "+err.Error(), "invalid_request_error", "")
			return
		}
		f.From = t
	}

	if s := q.Get("to"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to: "+err.Error(), "invalid_request_error", "")
			return
		}
		f.To = t
	}

	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 1000 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 1000", "invalid_request_error", "")
			return
		}
		f.Limit = n
	}

	if s := q.Get("offset"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "offset must be >= 0", "invalid_request_error", "")
			return
		}
		f.Offset = n
	}

	entries, err := h.store.List(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query logs", "api_error", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   entries,
	})
}
