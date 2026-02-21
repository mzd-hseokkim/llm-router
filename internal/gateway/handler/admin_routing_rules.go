package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/gateway/types"
)

// routingRuleStore is the persistence interface for routing rules.
type routingRuleStore interface {
	List(ctx context.Context) ([]types.RouteRule, error)
	Get(ctx context.Context, id uuid.UUID) (*types.RouteRule, error)
	Create(ctx context.Context, rule *types.RouteRule) error
	Update(ctx context.Context, rule *types.RouteRule) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// routingEngineReloader is the interface for reloading the advanced routing engine.
type routingEngineReloader interface {
	Reload(rules []types.RouteRule) error
}

// AdminRoutingRulesHandler handles CRUD for advanced routing rules + dry-run.
type AdminRoutingRulesHandler struct {
	store  routingRuleStore
	engine routingEngineReloader // may be nil if advanced routing is disabled
}

// NewAdminRoutingRulesHandler creates the handler.
func NewAdminRoutingRulesHandler(store routingRuleStore, engine routingEngineReloader) *AdminRoutingRulesHandler {
	return &AdminRoutingRulesHandler{store: store, engine: engine}
}

// List returns all routing rules.
func (h *AdminRoutingRulesHandler) List(w http.ResponseWriter, r *http.Request) {
	rules, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list rules", "api_error", "")
		return
	}
	if rules == nil {
		rules = []types.RouteRule{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rules})
}

// Create adds a new routing rule and reloads the engine.
func (h *AdminRoutingRulesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var rule types.RouteRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error", "")
		return
	}
	if rule.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "invalid_request_error", "")
		return
	}
	if len(rule.Targets) == 0 {
		writeError(w, http.StatusBadRequest, "targets must not be empty", "invalid_request_error", "")
		return
	}

	if err := h.store.Create(r.Context(), &rule); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	h.reloadEngine(r.Context())
	writeJSON(w, http.StatusCreated, rule)
}

// UpdateRule replaces a routing rule and reloads the engine.
func (h *AdminRoutingRulesHandler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id", "invalid_request_error", "")
		return
	}

	var rule types.RouteRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error", "")
		return
	}
	rule.ID = id

	if err := h.store.Update(r.Context(), &rule); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	h.reloadEngine(r.Context())
	writeJSON(w, http.StatusOK, rule)
}

// DeleteRule removes a routing rule and reloads the engine.
func (h *AdminRoutingRulesHandler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id", "invalid_request_error", "")
		return
	}
	if err := h.store.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	h.reloadEngine(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// ReloadRules forces the engine to reload rules from DB.
func (h *AdminRoutingRulesHandler) ReloadRules(w http.ResponseWriter, r *http.Request) {
	if err := h.reloadEngine(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reloaded"})
}

// DryRun simulates the routing decision without sending a real request.
func (h *AdminRoutingRulesHandler) DryRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model    string            `json:"model"`
		Metadata map[string]string `json:"metadata"`
		Messages []types.Message   `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error", "")
		return
	}

	rules, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}

	// Find first matching rule (simple in-process simulation).
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if rule.Match.Model != "" && rule.Match.Model != req.Model {
			continue
		}
		if len(rule.Match.Metadata) > 0 {
			matched := true
			for k, v := range rule.Match.Metadata {
				if req.Metadata[k] != v {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
		}
		// Matched — return result.
		writeJSON(w, http.StatusOK, map[string]any{
			"matched_rule": rule.Name,
			"strategy":     rule.Strategy,
			"targets":      rule.Targets,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"matched_rule": nil,
		"message":      "no matching rule found; default routing applies",
	})
}

func (h *AdminRoutingRulesHandler) reloadEngine(ctx context.Context) error {
	if h.engine == nil {
		return nil
	}
	rules, err := h.store.List(ctx)
	if err != nil {
		return err
	}
	return h.engine.Reload(rules)
}
