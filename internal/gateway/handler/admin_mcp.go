package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/llm-router/gateway/internal/mcp"
)

// AdminMCPHandler handles /admin/mcp/* endpoints.
type AdminMCPHandler struct {
	hub    *mcp.Hub
	cfgs   []mcp.ServerConfig // registered server configs (for CRUD)
	logger *slog.Logger
}

// NewAdminMCPHandler creates an AdminMCPHandler.
func NewAdminMCPHandler(hub *mcp.Hub, cfgs []mcp.ServerConfig, logger *slog.Logger) *AdminMCPHandler {
	return &AdminMCPHandler{hub: hub, cfgs: cfgs, logger: logger}
}

// ListServers handles GET /admin/mcp/servers.
func (h *AdminMCPHandler) ListServers(w http.ResponseWriter, r *http.Request) {
	type serverInfo struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		URL    string `json:"url,omitempty"`
		Status string `json:"status"`
	}

	health := h.hub.HealthAll(r.Context())
	infos := make([]serverInfo, 0, len(h.cfgs))
	for _, cfg := range h.cfgs {
		status := health[cfg.Name]
		if status == "" {
			status = "unknown"
		}
		infos = append(infos, serverInfo{
			Name:   cfg.Name,
			Type:   cfg.Type,
			URL:    cfg.URL,
			Status: status,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": infos})
}

// RegisterServer handles POST /admin/mcp/servers.
// Registers a new MCP server at runtime (config is not persisted — use config file for persistence).
func (h *AdminMCPHandler) RegisterServer(w http.ResponseWriter, r *http.Request) {
	var cfg mcp.ServerConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if cfg.Name == "" || cfg.Type == "" {
		http.Error(w, "name and type are required", http.StatusBadRequest)
		return
	}

	s := mcp.NewServer(cfg)
	if err := s.Connect(r.Context()); err != nil {
		h.logger.Error("admin mcp: connect failed", "server", cfg.Name, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	h.hub.Register(s)
	h.cfgs = append(h.cfgs, cfg)

	writeJSON(w, http.StatusCreated, map[string]any{
		"name":    cfg.Name,
		"message": "server registered and connected",
	})
}

// GetServer handles GET /admin/mcp/servers/{name}.
func (h *AdminMCPHandler) GetServer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	for _, cfg := range h.cfgs {
		if cfg.Name == name {
			status := "unknown"
			if health := h.hub.HealthAll(context.Background()); health != nil {
				if s, ok := health[name]; ok {
					status = s
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"name":   cfg.Name,
				"type":   cfg.Type,
				"url":    cfg.URL,
				"status": status,
			})
			return
		}
	}
	http.Error(w, "server not found", http.StatusNotFound)
}

// DeleteServer handles DELETE /admin/mcp/servers/{name}.
func (h *AdminMCPHandler) DeleteServer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	s, err := h.hub.Server(name)
	if err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	if err := s.Close(); err != nil {
		h.logger.Warn("admin mcp: close error", "server", name, "error", err)
	}

	// Remove from cfgs.
	newCfgs := h.cfgs[:0]
	for _, cfg := range h.cfgs {
		if cfg.Name != name {
			newCfgs = append(newCfgs, cfg)
		}
	}
	h.cfgs = newCfgs

	writeJSON(w, http.StatusOK, map[string]any{"message": "server removed"})
}

// HealthCheck handles GET /admin/mcp/servers/{name}/health.
func (h *AdminMCPHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	s, err := h.hub.Server(name)
	if err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	if err := s.Health(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"name":   name,
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "status": "ok"})
}

// ListServerTools handles GET /admin/mcp/servers/{name}/tools.
func (h *AdminMCPHandler) ListServerTools(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	s, err := h.hub.Server(name)
	if err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	tools, err := s.ListTools(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

// SetPolicy handles POST /admin/mcp/policies.
// Policies are virtual-key specific — this endpoint is for documentation/testing.
func (h *AdminMCPHandler) SetPolicy(w http.ResponseWriter, r *http.Request) {
	var policy mcp.Policy
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// In a production implementation, policies would be stored per virtual key.
	// Here we acknowledge the schema is correct.
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "policy schema valid; apply via virtual key metadata",
		"policy":  policy,
	})
}
