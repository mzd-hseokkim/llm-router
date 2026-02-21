package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/llm-router/gateway/internal/billing"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

// AdminReportsHandler serves /admin/reports/* and /admin/billing/* endpoints.
type AdminReportsHandler struct {
	svc          *billing.ChargebackService
	billingStore *pgstore.BillingStore
}

// NewAdminReportsHandler returns a new handler.
func NewAdminReportsHandler(svc *billing.ChargebackService, billingStore *pgstore.BillingStore) *AdminReportsHandler {
	return &AdminReportsHandler{svc: svc, billingStore: billingStore}
}

// Chargeback handles GET /admin/reports/chargeback?period=2026-01&format=json|csv.
func (h *AdminReportsHandler) Chargeback(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parsePeriodParam(w, r)
	if !ok {
		return
	}

	report, err := h.svc.BuildReport(r.Context(), from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}

	format := r.URL.Query().Get("format")
	if format == "csv" {
		data, err := report.ToCSV()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "CSV generation failed", "api_error", "")
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="chargeback-`+report.Period+`.csv"`)
		_, _ = w.Write(data)
		return
	}

	writeJSON(w, http.StatusOK, report)
}

// Showback handles GET /admin/reports/showback?period=2026-01&team_id=uuid.
func (h *AdminReportsHandler) Showback(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parsePeriodParam(w, r)
	if !ok {
		return
	}
	teamID := r.URL.Query().Get("team_id")
	if teamID == "" {
		writeError(w, http.StatusBadRequest, "team_id is required", "invalid_request_error", "")
		return
	}

	report, err := h.svc.ShowbackReport(r.Context(), teamID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// GetMarkup handles GET /admin/billing/markup?team_id=uuid.
func (h *AdminReportsHandler) GetMarkup(w http.ResponseWriter, r *http.Request) {
	var teamID *string
	if t := r.URL.Query().Get("team_id"); t != "" {
		teamID = &t
	}
	cfg, err := h.billingStore.GetMarkupConfig(r.Context(), teamID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// UpsertMarkup handles PUT /admin/billing/markup.
func (h *AdminReportsHandler) UpsertMarkup(w http.ResponseWriter, r *http.Request) {
	var cfg billing.MarkupConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error", "")
		return
	}
	if err := h.billingStore.UpsertMarkupConfig(r.Context(), &cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// parsePeriodParam parses ?period=2026-01 into (from, to) time boundaries.
// Returns false and writes the error response when invalid.
func parsePeriodParam(w http.ResponseWriter, r *http.Request) (from, to time.Time, ok bool) {
	period := r.URL.Query().Get("period")
	if period == "" {
		// Default: current month.
		now := time.Now().UTC()
		period = now.Format("2006-01")
	}

	t, err := time.Parse("2006-01", period)
	if err != nil {
		writeError(w, http.StatusBadRequest, "period must be YYYY-MM (e.g. 2026-01)", "invalid_request_error", "")
		return time.Time{}, time.Time{}, false
	}
	from = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	to = from.AddDate(0, 1, 0)
	return from, to, true
}
