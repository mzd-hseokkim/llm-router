package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/llm-router/gateway/internal/auth"
	"github.com/llm-router/gateway/internal/ratelimit"
)

// AdminRateLimitsHandler provides admin endpoints for rate limit inspection.
type AdminRateLimitsHandler struct {
	keyStore auth.Store
	limiter  ratelimit.Limiter
}

// NewAdminRateLimitsHandler creates the handler.
func NewAdminRateLimitsHandler(keyStore auth.Store, limiter ratelimit.Limiter) *AdminRateLimitsHandler {
	return &AdminRateLimitsHandler{keyStore: keyStore, limiter: limiter}
}

// Get returns the rate limit configuration for a virtual key.
// GET /admin/rate-limits/{id}
func (h *AdminRateLimitsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	key, err := h.keyStore.GetByID(r.Context(), id)
	if err != nil || key == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "key not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"key_id":       key.ID.String(),
		"rpm_limit":    key.RPMLimit,
		"tpm_limit":    key.TPMLimit,
		"rpm_window":   ratelimit.WindowMinute(),
	})
}

// Reset returns when the current rate limit window expires (windows self-expire).
// POST /admin/rate-limits/{id}/reset
func (h *AdminRateLimitsHandler) Reset(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	key, err := h.keyStore.GetByID(r.Context(), id)
	if err != nil || key == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "key not found"})
		return
	}

	minute := ratelimit.WindowMinute()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"key_id":   key.ID.String(),
		"rpm_key":  ratelimit.KeyForKeyRPM(key.ID.String(), minute),
		"tpm_key":  ratelimit.KeyForKeyTPM(key.ID.String(), minute),
		"note":     "rate limit windows reset on the next minute boundary automatically",
		"reset_at": time.Now().UTC().Truncate(time.Minute).Add(time.Minute).Format(time.RFC3339),
	})
}
