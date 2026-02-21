package handler

import (
	"net/http"
	"time"

	"github.com/llm-router/gateway/internal/billing"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

// BillingAPIHandler serves the external billing API (/api/billing/*).
type BillingAPIHandler struct {
	billingStore *pgstore.BillingStore
}

// NewBillingAPIHandler returns a new handler.
func NewBillingAPIHandler(billingStore *pgstore.BillingStore) *BillingAPIHandler {
	return &BillingAPIHandler{billingStore: billingStore}
}

// Usage handles GET /api/billing/usage?from=2026-01-01&to=2026-01-31.
// Intended for integration with external billing systems (Stripe, SAP, etc.).
func (h *BillingAPIHandler) Usage(w http.ResponseWriter, r *http.Request) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	var from, to time.Time
	var err error

	if fromStr == "" || toStr == "" {
		now := time.Now().UTC()
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		to = from.AddDate(0, 1, 0)
	} else {
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from must be YYYY-MM-DD", "invalid_request_error", "")
			return
		}
		to, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to must be YYYY-MM-DD", "invalid_request_error", "")
			return
		}
	}

	items, err := h.billingStore.GetBillingUsageItems(r.Context(), from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	if items == nil {
		items = []*billing.BillingUsageItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
