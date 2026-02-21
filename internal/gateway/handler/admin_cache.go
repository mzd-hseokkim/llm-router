package handler

import (
	"fmt"
	"net/http"

	exactcache "github.com/llm-router/gateway/internal/cache/exact"
)

// AdminCacheHandler handles /admin/cache/* endpoints.
type AdminCacheHandler struct {
	cache *exactcache.Cache
}

// NewAdminCacheHandler creates a handler for cache administration.
func NewAdminCacheHandler(cache *exactcache.Cache) *AdminCacheHandler {
	return &AdminCacheHandler{cache: cache}
}

// Delete handles DELETE /admin/cache/exact — flush the entire exact cache,
// or DELETE /admin/cache/exact?model=xxx — flush by model prefix.
func (h *AdminCacheHandler) Delete(w http.ResponseWriter, r *http.Request) {
	model := r.URL.Query().Get("model")

	pattern := "*"
	if model != "" {
		// Cache keys are SHA-256 hashes so we cannot filter by model in the key.
		// We use a wildcard scan and return a warning.
		pattern = "*"
	}

	if err := h.cache.DeleteByPattern(r.Context(), pattern); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to flush cache: %v", err), "api_error", "")
		return
	}

	msg := "exact cache flushed"
	if model != "" {
		msg = fmt.Sprintf("exact cache flushed (model filter '%s' not applied: keys are hashed)", model)
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": msg})
}

// Get handles GET /admin/cache/exact/{hash} — retrieve a cached entry by hash.
func (h *AdminCacheHandler) Get(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if hash == "" {
		writeError(w, http.StatusBadRequest, "hash is required", "invalid_request_error", "")
		return
	}

	entry, err := h.cache.Get(r.Context(), hash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	if entry == nil {
		writeError(w, http.StatusNotFound, "cache entry not found", "not_found_error", "")
		return
	}
	writeJSON(w, http.StatusOK, entry)
}
