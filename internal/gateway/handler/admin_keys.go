package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/auth"
)

// AdminKeysHandler handles CRUD for virtual keys via /admin/keys.
type AdminKeysHandler struct {
	store  auth.Store
	cache  auth.Cache
	logger *slog.Logger
}

// NewAdminKeysHandler creates a new handler.
func NewAdminKeysHandler(store auth.Store, cache auth.Cache, logger *slog.Logger) *AdminKeysHandler {
	return &AdminKeysHandler{store: store, cache: cache, logger: logger}
}

// createKeyRequest is the JSON body for POST /admin/keys.
type createKeyRequest struct {
	Name          string          `json:"name"`
	ExpiresAt     *time.Time      `json:"expires_at,omitempty"`
	BudgetUSD     *float64        `json:"budget_usd,omitempty"`
	RPMLimit      *int            `json:"rpm_limit,omitempty"`
	TPMLimit      *int            `json:"tpm_limit,omitempty"`
	AllowedModels []string        `json:"allowed_models,omitempty"`
	BlockedModels []string        `json:"blocked_models,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
}

// createKeyResponse includes the raw key (returned once only).
type createKeyResponse struct {
	ID        uuid.UUID `json:"id"`
	Key       string    `json:"key"` // raw key — shown once
	KeyPrefix string    `json:"key_prefix"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// keyResponse is the view model used for list/get responses (no raw key).
type keyResponse struct {
	ID            uuid.UUID       `json:"id"`
	KeyPrefix     string          `json:"key_prefix"`
	Name          string          `json:"name"`
	ExpiresAt     *time.Time      `json:"expires_at,omitempty"`
	BudgetUSD     *float64        `json:"budget_usd,omitempty"`
	RPMLimit      *int            `json:"rpm_limit,omitempty"`
	TPMLimit      *int            `json:"tpm_limit,omitempty"`
	AllowedModels []string        `json:"allowed_models,omitempty"`
	BlockedModels []string        `json:"blocked_models,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	IsActive      bool            `json:"is_active"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	LastUsedAt    *time.Time      `json:"last_used_at,omitempty"`
}

// updateKeyRequest is the JSON body for PATCH /admin/keys/{id}.
type updateKeyRequest struct {
	Name          *string         `json:"name,omitempty"`
	ExpiresAt     *time.Time      `json:"expires_at,omitempty"`
	BudgetUSD     *float64        `json:"budget_usd,omitempty"`
	RPMLimit      *int            `json:"rpm_limit,omitempty"`
	TPMLimit      *int            `json:"tpm_limit,omitempty"`
	AllowedModels []string        `json:"allowed_models,omitempty"`
	BlockedModels []string        `json:"blocked_models,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	IsActive      *bool           `json:"is_active,omitempty"`
}

// Create handles POST /admin/keys.
func (h *AdminKeysHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error", "")
		return
	}

	rawKey, keyHash, keyPrefix, err := auth.GenerateKey()
	if err != nil {
		h.logger.Error("failed to generate virtual key", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate key", "internal_error", "")
		return
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = json.RawMessage("{}")
	}

	key := &auth.VirtualKey{
		KeyHash:       keyHash,
		KeyPrefix:     keyPrefix,
		Name:          req.Name,
		ExpiresAt:     req.ExpiresAt,
		BudgetUSD:     req.BudgetUSD,
		RPMLimit:      req.RPMLimit,
		TPMLimit:      req.TPMLimit,
		AllowedModels: req.AllowedModels,
		BlockedModels: req.BlockedModels,
		Metadata:      metadata,
		IsActive:      true,
	}

	if err := h.store.Create(r.Context(), key); err != nil {
		h.logger.Error("failed to create virtual key", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create key", "internal_error", "")
		return
	}

	writeJSON(w, http.StatusCreated, createKeyResponse{
		ID:        key.ID,
		Key:       rawKey,
		KeyPrefix: key.KeyPrefix,
		Name:      key.Name,
		CreatedAt: key.CreatedAt,
	})
}

// List handles GET /admin/keys.
func (h *AdminKeysHandler) List(w http.ResponseWriter, r *http.Request) {
	keys, err := h.store.List(r.Context())
	if err != nil {
		h.logger.Error("failed to list virtual keys", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list keys", "internal_error", "")
		return
	}

	resp := make([]keyResponse, 0, len(keys))
	for _, k := range keys {
		resp = append(resp, toKeyResponse(k))
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": resp})
}

// Get handles GET /admin/keys/{id}.
func (h *AdminKeysHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	key, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, auth.ErrKeyNotFound) {
			writeError(w, http.StatusNotFound, "key not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to get virtual key", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get key", "internal_error", "")
		return
	}

	writeJSON(w, http.StatusOK, toKeyResponse(key))
}

// Update handles PATCH /admin/keys/{id}.
func (h *AdminKeysHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	key, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, auth.ErrKeyNotFound) {
			writeError(w, http.StatusNotFound, "key not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to get virtual key for update", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get key", "internal_error", "")
		return
	}

	var req updateKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error", "")
		return
	}

	// Apply only provided fields.
	if req.Name != nil {
		key.Name = *req.Name
	}
	if req.ExpiresAt != nil {
		key.ExpiresAt = req.ExpiresAt
	}
	if req.BudgetUSD != nil {
		key.BudgetUSD = req.BudgetUSD
	}
	if req.RPMLimit != nil {
		key.RPMLimit = req.RPMLimit
	}
	if req.TPMLimit != nil {
		key.TPMLimit = req.TPMLimit
	}
	if req.AllowedModels != nil {
		key.AllowedModels = req.AllowedModels
	}
	if req.BlockedModels != nil {
		key.BlockedModels = req.BlockedModels
	}
	if req.Metadata != nil {
		key.Metadata = req.Metadata
	}
	if req.IsActive != nil {
		key.IsActive = *req.IsActive
	}

	if err := h.store.Update(r.Context(), key); err != nil {
		h.logger.Error("failed to update virtual key", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update key", "internal_error", "")
		return
	}

	// Invalidate cache so the updated key is picked up immediately.
	if cacheErr := h.cache.Delete(r.Context(), key.KeyHash); cacheErr != nil {
		h.logger.Warn("failed to invalidate key cache after update", "error", cacheErr)
	}

	writeJSON(w, http.StatusOK, toKeyResponse(key))
}

// Deactivate handles DELETE /admin/keys/{id}.
func (h *AdminKeysHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	// Fetch first so we can invalidate the cache by hash.
	key, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, auth.ErrKeyNotFound) {
			writeError(w, http.StatusNotFound, "key not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to get virtual key for deactivation", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get key", "internal_error", "")
		return
	}

	if err := h.store.Deactivate(r.Context(), id); err != nil {
		h.logger.Error("failed to deactivate virtual key", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to deactivate key", "internal_error", "")
		return
	}

	// Invalidate cache immediately.
	if cacheErr := h.cache.Delete(r.Context(), key.KeyHash); cacheErr != nil {
		h.logger.Warn("failed to invalidate key cache after deactivation", "error", cacheErr)
	}

	w.WriteHeader(http.StatusNoContent)
}

// Regenerate handles POST /admin/keys/{id}/regenerate — rotates the raw key.
func (h *AdminKeysHandler) Regenerate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	key, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, auth.ErrKeyNotFound) {
			writeError(w, http.StatusNotFound, "key not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to get key for regeneration", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get key", "internal_error", "")
		return
	}

	oldHash := key.KeyHash
	rawKey, keyHash, keyPrefix, err := auth.GenerateKey()
	if err != nil {
		h.logger.Error("failed to generate new key", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate key", "internal_error", "")
		return
	}

	if err := h.store.UpdateHash(r.Context(), id, keyHash, keyPrefix); err != nil {
		h.logger.Error("failed to update key hash", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to regenerate key", "internal_error", "")
		return
	}

	// Invalidate the old cache entry.
	if cacheErr := h.cache.Delete(r.Context(), oldHash); cacheErr != nil {
		h.logger.Warn("failed to invalidate old key cache", "error", cacheErr)
	}

	writeJSON(w, http.StatusOK, createKeyResponse{
		ID:        key.ID,
		Key:       rawKey,
		KeyPrefix: keyPrefix,
		Name:      key.Name,
		CreatedAt: key.CreatedAt,
	})
}

// --- helpers ---

func toKeyResponse(k *auth.VirtualKey) keyResponse {
	return keyResponse{
		ID:            k.ID,
		KeyPrefix:     k.KeyPrefix,
		Name:          k.Name,
		ExpiresAt:     k.ExpiresAt,
		BudgetUSD:     k.BudgetUSD,
		RPMLimit:      k.RPMLimit,
		TPMLimit:      k.TPMLimit,
		AllowedModels: k.AllowedModels,
		BlockedModels: k.BlockedModels,
		Metadata:      k.Metadata,
		IsActive:      k.IsActive,
		CreatedAt:     k.CreatedAt,
		UpdatedAt:     k.UpdatedAt,
		LastUsedAt:    k.LastUsedAt,
	}
}

func parseUUID(w http.ResponseWriter, s string) (uuid.UUID, bool) {
	id, err := uuid.Parse(s)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key ID", "invalid_request_error", "")
		return uuid.UUID{}, false
	}
	return id, true
}
