package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/llm-router/gateway/internal/gateway/circuitbreaker"
)

// AdminCircuitBreakerHandler handles /admin/circuit-breakers endpoints.
type AdminCircuitBreakerHandler struct {
	cb     *circuitbreaker.CircuitBreaker
	logger *slog.Logger
}

// NewAdminCircuitBreakerHandler creates an AdminCircuitBreakerHandler.
func NewAdminCircuitBreakerHandler(cb *circuitbreaker.CircuitBreaker, logger *slog.Logger) *AdminCircuitBreakerHandler {
	return &AdminCircuitBreakerHandler{cb: cb, logger: logger}
}

// List returns the state of all tracked circuit breakers.
func (h *AdminCircuitBreakerHandler) List(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"circuit_breakers": h.cb.AllStatus(),
	})
}

// Reset manually closes the circuit for a specific provider.
func (h *AdminCircuitBreakerHandler) Reset(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")
	if providerName == "" {
		writeError(w, http.StatusBadRequest, "provider is required", "invalid_request_error", "")
		return
	}

	h.cb.Reset(providerName)
	h.logger.Info("circuit breaker reset via admin", "provider", providerName)

	writeJSON(w, http.StatusOK, map[string]any{
		"provider": providerName,
		"state":    "closed",
		"message":  "circuit breaker reset successfully",
	})
}
