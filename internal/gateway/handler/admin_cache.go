package handler

import (
	"fmt"
	"net/http"
	"time"

	exactcache "github.com/llm-router/gateway/internal/cache/exact"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

// AdminCacheHandler handles /admin/cache/* endpoints.
type AdminCacheHandler struct {
	cache    *exactcache.Cache
	logStore *pgstore.LogStore
}

// NewAdminCacheHandler creates a handler for cache administration.
func NewAdminCacheHandler(cache *exactcache.Cache, logStore *pgstore.LogStore) *AdminCacheHandler {
	return &AdminCacheHandler{cache: cache, logStore: logStore}
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

// Stats handles GET /admin/cache/stats — return today's cache hit/miss counts.
func (h *AdminCacheHandler) Stats(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	total, hits, err := h.logStore.CacheStats(r.Context(), dayStart, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to query cache stats: %v", err), "api_error", "")
		return
	}

	var hitRate *float64
	if total > 0 {
		rate := float64(hits) / float64(total) * 100
		hitRate = &rate
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total":    total,
		"hits":     hits,
		"hit_rate": hitRate,
	})
}
