package handler

import (
	"net/http"
	"time"

	"github.com/llm-router/gateway/internal/health"
	"github.com/llm-router/gateway/internal/provider"
	"github.com/llm-router/gateway/internal/telemetry"
)

// HealthHandler handles /health/* endpoints.
type HealthHandler struct {
	checker  *health.Checker
	tracker  *health.ProviderTracker
	registry *provider.Registry
}

// NewHealthHandler creates a HealthHandler.
func NewHealthHandler(checker *health.Checker, tracker *health.ProviderTracker, registry *provider.Registry) *HealthHandler {
	return &HealthHandler{checker: checker, tracker: tracker, registry: registry}
}

// Live handles GET /health/live — always 200 while the process is running.
func (h *HealthHandler) Live(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready handles GET /health/ready — 200 if DB and Redis are reachable, 503 otherwise.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	checks := h.checker.ReadyChecks(r.Context())

	allOK := true
	for _, cr := range checks {
		if cr.Status != "ok" {
			allOK = false
			break
		}
	}

	deps := make(map[string]string, len(checks))
	for k, v := range checks {
		deps[k] = v.Status
	}

	status := "ready"
	code := http.StatusOK
	if !allOK {
		status = "not_ready"
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, map[string]any{
		"status": status,
		"checks": deps,
	})
}

// Full handles GET /health — detailed status including providers.
func (h *HealthHandler) Full(w http.ResponseWriter, r *http.Request) {
	result := h.checker.Check(r.Context())
	providerStatus := h.buildProviderStatus()

	// Overall status is "degraded" if any provider is degraded/unhealthy but DB/Redis are fine.
	if result.Status == "ok" {
		for _, ps := range providerStatus {
			if s, ok := ps.(map[string]any); ok {
				if s["status"] == "unhealthy" {
					result.Status = "degraded"
					break
				}
				if s["status"] == "degraded" {
					result.Status = "degraded"
				}
			}
		}
	}

	// Update ProviderHealth Prometheus gauges.
	for name, ps := range providerStatus {
		if s, ok := ps.(map[string]any); ok {
			val := 1.0
			if s["status"] != "ok" {
				val = 0.0
			}
			telemetry.ProviderHealth.WithLabelValues(name).Set(val)
		}
	}

	writeJSON(w, httpStatusForHealth(result.Status), map[string]any{
		"status":         result.Status,
		"version":        result.Version,
		"uptime_seconds": result.UptimeSeconds,
		"timestamp":      result.Timestamp.Format(time.RFC3339),
		"checks":         result.Checks,
		"providers":      providerStatus,
	})
}

// Providers handles GET /health/providers — provider-only status.
func (h *HealthHandler) Providers(w http.ResponseWriter, r *http.Request) {
	providerStatus := h.buildProviderStatus()
	writeJSON(w, http.StatusOK, map[string]any{"providers": providerStatus})
}

func (h *HealthHandler) buildProviderStatus() map[string]any {
	providerNames := h.registry.AllProviders()
	out := make(map[string]any, len(providerNames))
	for _, name := range providerNames {
		status := h.tracker.Status(name)
		errorRate := h.tracker.ErrorRate(name)
		out[name] = map[string]any{
			"status":         status,
			"error_rate_1m":  errorRate,
		}
	}
	return out
}

func httpStatusForHealth(status string) int {
	switch status {
	case "unhealthy":
		return http.StatusServiceUnavailable
	default:
		return http.StatusOK
	}
}
