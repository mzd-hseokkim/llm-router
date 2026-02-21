package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/llm-router/gateway/internal/residency"
)

// AdminResidencyHandler serves /admin/data-residency/* endpoints.
type AdminResidencyHandler struct {
	enforcer *residency.Enforcer
	registry *residency.Registry
}

// NewAdminResidencyHandler returns a new AdminResidencyHandler.
func NewAdminResidencyHandler(registry *residency.Registry, enforcer *residency.Enforcer) *AdminResidencyHandler {
	return &AdminResidencyHandler{enforcer: enforcer, registry: registry}
}

// listPoliciesResponse is the JSON shape for a single policy in the list.
type listPoliciesResponse struct {
	Name             string   `json:"name"`
	AllowedProviders []string `json:"allowed_providers"`
	BlockedProviders []string `json:"blocked_providers"`
	AllowedRegions   []string `json:"allowed_regions"`
}

// List handles GET /admin/data-residency/policies
func (h *AdminResidencyHandler) List(w http.ResponseWriter, r *http.Request) {
	policies := h.registry.List()
	out := make([]listPoliciesResponse, 0, len(policies))
	for _, p := range policies {
		blocked := make([]string, 0, len(p.BlockedProviders))
		for name := range p.BlockedProviders {
			blocked = append(blocked, name)
		}
		out = append(out, listPoliciesResponse{
			Name:             p.Name,
			AllowedProviders: p.AllowedProviderNames(),
			BlockedProviders: blocked,
			AllowedRegions:   p.AllowedRegions,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"policies": out})
}

// Get handles GET /admin/data-residency/policies/{name}
func (h *AdminResidencyHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	policy, ok := h.registry.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "policy not found", "not_found", "")
		return
	}
	blocked := make([]string, 0, len(policy.BlockedProviders))
	for n := range policy.BlockedProviders {
		blocked = append(blocked, n)
	}
	writeJSON(w, http.StatusOK, listPoliciesResponse{
		Name:             policy.Name,
		AllowedProviders: policy.AllowedProviderNames(),
		BlockedProviders: blocked,
		AllowedRegions:   policy.AllowedRegions,
	})
}

// validateRequest is the body for POST /admin/data-residency/validate
type validateRequest struct {
	Policy   string `json:"policy"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// Validate handles POST /admin/data-residency/validate
func (h *AdminResidencyHandler) Validate(w http.ResponseWriter, r *http.Request) {
	var req validateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error", "")
		return
	}
	req.Policy = strings.TrimSpace(req.Policy)
	req.Provider = strings.TrimSpace(req.Provider)
	if req.Policy == "" || req.Provider == "" {
		writeError(w, http.StatusBadRequest, "policy and provider are required", "invalid_request_error", "")
		return
	}

	if err := h.enforcer.CheckProvider(req.Policy, req.Provider, req.Model); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"compliant": false,
			"reason":    err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"compliant": true,
	})
}

// Report handles GET /admin/data-residency/report
func (h *AdminResidencyHandler) Report(w http.ResponseWriter, r *http.Request) {
	policies := h.registry.List()
	report := make([]map[string]any, 0, len(policies))
	for _, p := range policies {
		allowed := p.AllowedProviderNames()
		blocked := make([]string, 0, len(p.BlockedProviders))
		for n := range p.BlockedProviders {
			blocked = append(blocked, n)
		}
		report = append(report, map[string]any{
			"policy":            p.Name,
			"allowed_providers": allowed,
			"blocked_providers": blocked,
			"allowed_regions":   p.AllowedRegions,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"report":       report,
		"policy_count": len(policies),
	})
}
