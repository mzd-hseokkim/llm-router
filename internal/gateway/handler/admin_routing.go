package handler

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/llm-router/gateway/internal/config"
)

// RoutingStore holds the current routing configuration in memory and supports
// hot-reload via registered listener callbacks.
type RoutingStore struct {
	mu        sync.RWMutex
	cfg       config.RoutingConfig
	listeners []func(config.RoutingConfig)
}

// NewRoutingStore creates a RoutingStore seeded with the given config.
func NewRoutingStore(initial config.RoutingConfig) *RoutingStore {
	return &RoutingStore{cfg: initial}
}

// Get returns the current routing configuration (read-only copy).
func (s *RoutingStore) Get() config.RoutingConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Set replaces the routing configuration and notifies all listeners.
func (s *RoutingStore) Set(cfg config.RoutingConfig) {
	s.mu.Lock()
	s.cfg = cfg
	listeners := make([]func(config.RoutingConfig), len(s.listeners))
	copy(listeners, s.listeners)
	s.mu.Unlock()

	for _, fn := range listeners {
		fn(cfg)
	}
}

// Subscribe registers a callback that is called whenever the config changes.
func (s *RoutingStore) Subscribe(fn func(config.RoutingConfig)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

// AdminRoutingHandler handles routing config CRUD.
type AdminRoutingHandler struct {
	store *RoutingStore
}

// NewAdminRoutingHandler creates the handler.
func NewAdminRoutingHandler(store *RoutingStore) *AdminRoutingHandler {
	return &AdminRoutingHandler{store: store}
}

// Get handles GET /admin/routing — returns current routing config.
func (h *AdminRoutingHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := h.store.Get()
	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /admin/routing — replaces the routing config.
func (h *AdminRoutingHandler) Update(w http.ResponseWriter, r *http.Request) {
	var cfg config.RoutingConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid routing config: "+err.Error(), "invalid_request_error", "")
		return
	}
	h.store.Set(cfg)
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// Reload handles POST /admin/routing/reload — re-applies the current config to all subscribers.
func (h *AdminRoutingHandler) Reload(w http.ResponseWriter, r *http.Request) {
	cfg := h.store.Get()
	h.store.Set(cfg) // triggers all listeners with the same config
	writeJSON(w, http.StatusOK, map[string]string{"status": "reloaded"})
}
