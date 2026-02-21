package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

// AdminUsageHandler provides usage summary endpoints.
type AdminUsageHandler struct {
	usage *pgstore.UsageStore
}

// NewAdminUsageHandler creates the handler.
func NewAdminUsageHandler(usage *pgstore.UsageStore) *AdminUsageHandler {
	return &AdminUsageHandler{usage: usage}
}

// Summary returns aggregated usage for an entity over a period.
// GET /admin/usage/summary?entity_type=key&entity_id=uuid&period=monthly
func (h *AdminUsageHandler) Summary(w http.ResponseWriter, r *http.Request) {
	entityType := r.URL.Query().Get("entity_type")
	entityIDStr := r.URL.Query().Get("entity_id")
	period := r.URL.Query().Get("period")

	if entityType == "" || entityIDStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "entity_type and entity_id required"})
		return
	}

	entityID, err := uuid.Parse(entityIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid entity_id"})
		return
	}

	from, to := periodRange(period)

	summary, err := h.usage.GetSummary(r.Context(), entityType, entityID, from, to)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	byModel, err := h.usage.GetByModel(r.Context(), entityType, entityID, from, to)
	if err != nil {
		byModel = nil // non-fatal
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"entity_type":       entityType,
		"entity_id":         entityIDStr,
		"period":            period,
		"from":              from.Format(time.DateOnly),
		"to":                to.Format(time.DateOnly),
		"total_requests":    summary.TotalRequests,
		"total_tokens":      summary.TotalTokens,
		"prompt_tokens":     summary.PromptTokens,
		"completion_tokens": summary.CompletionTokens,
		"total_cost_usd":    summary.TotalCostUSD,
		"error_count":       summary.ErrorCount,
		"by_model":          byModel,
	})
}

// TopSpenders handles GET /admin/usage/top-spenders?limit=10&period=monthly
func (h *AdminUsageHandler) TopSpenders(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	limitStr := r.URL.Query().Get("limit")

	limit := 10
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	from, to := periodRange(period)
	spenders, err := h.usage.TopSpenders(r.Context(), from, to, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"period": period,
		"from":   from.Format(time.DateOnly),
		"to":     to.Format(time.DateOnly),
		"data":   spenders,
	})
}

// periodRange converts a period name to a [from, to] date range.
func periodRange(period string) (from, to time.Time) {
	now := time.Now().UTC()
	switch period {
	case "daily":
		from = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		to = from.AddDate(0, 0, 1).Add(-time.Second)
	case "weekly":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := now.AddDate(0, 0, -(weekday - 1))
		from = time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
		to = from.AddDate(0, 0, 7).Add(-time.Second)
	case "monthly":
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		to = from.AddDate(0, 1, 0).Add(-time.Second)
	default: // "daily" is default
		from = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		to = now
	}
	return from, to
}
