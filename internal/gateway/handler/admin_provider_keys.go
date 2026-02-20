package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/crypto"
	"github.com/llm-router/gateway/internal/provider"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

// AdminProviderKeysHandler handles CRUD for provider API keys via /admin/provider-keys.
type AdminProviderKeysHandler struct {
	store  provider.ProviderKeyStore
	km     *provider.KeyManager
	cipher *crypto.Cipher
	logger *slog.Logger
}

// NewAdminProviderKeysHandler creates a new handler.
func NewAdminProviderKeysHandler(
	store provider.ProviderKeyStore,
	km *provider.KeyManager,
	cipher *crypto.Cipher,
	logger *slog.Logger,
) *AdminProviderKeysHandler {
	return &AdminProviderKeysHandler{store: store, km: km, cipher: cipher, logger: logger}
}

// createProviderKeyRequest is the body for POST /admin/provider-keys.
type createProviderKeyRequest struct {
	Provider  string   `json:"provider"`
	KeyAlias  string   `json:"key_alias"`
	APIKey    string   `json:"api_key"` // encrypted then discarded
	GroupName string   `json:"group_name,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Weight    *int     `json:"weight,omitempty"`
	IsActive  *bool    `json:"is_active,omitempty"`
}

// updateProviderKeyRequest is the body for PUT /admin/provider-keys/:id.
type updateProviderKeyRequest struct {
	KeyAlias          *string  `json:"key_alias,omitempty"`
	GroupName         *string  `json:"group_name,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	Weight            *int     `json:"weight,omitempty"`
	IsActive          *bool    `json:"is_active,omitempty"`
	MonthlyBudgetUSD  *float64 `json:"monthly_budget_usd,omitempty"`
}

// rotateProviderKeyRequest is the body for PUT /admin/provider-keys/:id/rotate.
type rotateProviderKeyRequest struct {
	NewAPIKey string `json:"new_api_key"`
}

// providerKeyResponse is the view model; never includes the raw key.
type providerKeyResponse struct {
	ID                uuid.UUID  `json:"id"`
	Provider          string     `json:"provider"`
	KeyAlias          string     `json:"key_alias"`
	KeyPreview        string     `json:"key_preview"`
	GroupName         string     `json:"group_name,omitempty"`
	Tags              []string   `json:"tags,omitempty"`
	IsActive          bool       `json:"is_active"`
	Weight            int        `json:"weight"`
	MonthlyBudgetUSD  *float64   `json:"monthly_budget_usd,omitempty"`
	CurrentMonthSpend float64    `json:"current_month_spend"`
	UseCount          int64      `json:"use_count"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	LastUsedAt        *time.Time `json:"last_used_at,omitempty"`
}

// Create handles POST /admin/provider-keys.
func (h *AdminProviderKeysHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createProviderKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error", "")
		return
	}
	if req.Provider == "" || req.KeyAlias == "" || req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "provider, key_alias, and api_key are required", "invalid_request_error", "")
		return
	}

	encryptedKey, err := h.cipher.Encrypt([]byte(req.APIKey))
	if err != nil {
		h.logger.Error("failed to encrypt provider key", "error", err)
		writeError(w, http.StatusInternalServerError, "encryption failed", "internal_error", "")
		return
	}

	preview := keyPreview(req.APIKey)

	weight := 100
	if req.Weight != nil {
		weight = *req.Weight
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	rec := &provider.ProviderKeyRecord{
		Provider:     req.Provider,
		KeyAlias:     req.KeyAlias,
		EncryptedKey: encryptedKey,
		KeyPreview:   preview,
		GroupName:    req.GroupName,
		Tags:         req.Tags,
		Weight:       weight,
		IsActive:     isActive,
	}

	if err := h.store.Create(r.Context(), rec); err != nil {
		h.logger.Error("failed to create provider key", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create key", "internal_error", "")
		return
	}

	h.km.InvalidateCache(req.Provider)
	writeJSON(w, http.StatusCreated, toProviderKeyResponse(rec))
}

// List handles GET /admin/provider-keys and GET /admin/provider-keys?provider=xxx.
func (h *AdminProviderKeysHandler) List(w http.ResponseWriter, r *http.Request) {
	providerFilter := r.URL.Query().Get("provider")
	recs, err := h.store.List(r.Context(), providerFilter)
	if err != nil {
		h.logger.Error("failed to list provider keys", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list keys", "internal_error", "")
		return
	}

	resp := make([]providerKeyResponse, 0, len(recs))
	for _, rec := range recs {
		resp = append(resp, toProviderKeyResponse(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": resp})
}

// Get handles GET /admin/provider-keys/:id.
func (h *AdminProviderKeysHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	rec, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgstore.ErrProviderKeyNotFound) {
			writeError(w, http.StatusNotFound, "provider key not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to get provider key", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get key", "internal_error", "")
		return
	}

	writeJSON(w, http.StatusOK, toProviderKeyResponse(rec))
}

// Update handles PUT /admin/provider-keys/:id.
func (h *AdminProviderKeysHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	rec, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgstore.ErrProviderKeyNotFound) {
			writeError(w, http.StatusNotFound, "provider key not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to get provider key", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get key", "internal_error", "")
		return
	}

	var req updateProviderKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error", "")
		return
	}

	if req.KeyAlias != nil {
		rec.KeyAlias = *req.KeyAlias
	}
	if req.GroupName != nil {
		rec.GroupName = *req.GroupName
	}
	if req.Tags != nil {
		rec.Tags = req.Tags
	}
	if req.Weight != nil {
		rec.Weight = *req.Weight
	}
	if req.IsActive != nil {
		rec.IsActive = *req.IsActive
	}
	if req.MonthlyBudgetUSD != nil {
		rec.MonthlyBudgetUSD = req.MonthlyBudgetUSD
	}

	if err := h.store.Update(r.Context(), rec); err != nil {
		h.logger.Error("failed to update provider key", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update key", "internal_error", "")
		return
	}

	h.km.InvalidateCache(rec.Provider)
	writeJSON(w, http.StatusOK, toProviderKeyResponse(rec))
}

// Delete handles DELETE /admin/provider-keys/:id.
func (h *AdminProviderKeysHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	// Fetch first to get the provider name for cache invalidation.
	rec, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgstore.ErrProviderKeyNotFound) {
			writeError(w, http.StatusNotFound, "provider key not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to get provider key for deletion", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get key", "internal_error", "")
		return
	}

	if err := h.store.Delete(r.Context(), id); err != nil {
		h.logger.Error("failed to delete provider key", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete key", "internal_error", "")
		return
	}

	h.km.InvalidateCache(rec.Provider)
	w.WriteHeader(http.StatusNoContent)
}

// Rotate handles PUT /admin/provider-keys/:id/rotate.
func (h *AdminProviderKeysHandler) Rotate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	var req rotateProviderKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NewAPIKey == "" {
		writeError(w, http.StatusBadRequest, "new_api_key is required", "invalid_request_error", "")
		return
	}

	encryptedKey, err := h.cipher.Encrypt([]byte(req.NewAPIKey))
	if err != nil {
		h.logger.Error("failed to encrypt new provider key", "error", err)
		writeError(w, http.StatusInternalServerError, "encryption failed", "internal_error", "")
		return
	}

	preview := keyPreview(req.NewAPIKey)

	if err := h.store.RotateKey(r.Context(), id, encryptedKey, preview); err != nil {
		if errors.Is(err, pgstore.ErrProviderKeyNotFound) {
			writeError(w, http.StatusNotFound, "provider key not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to rotate provider key", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to rotate key", "internal_error", "")
		return
	}

	rec, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to fetch rotated provider key", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "key rotated but fetch failed", "internal_error", "")
		return
	}

	h.km.InvalidateCache(rec.Provider)
	writeJSON(w, http.StatusOK, toProviderKeyResponse(rec))
}

// --- helpers ---

func toProviderKeyResponse(rec *provider.ProviderKeyRecord) providerKeyResponse {
	return providerKeyResponse{
		ID:                rec.ID,
		Provider:          rec.Provider,
		KeyAlias:          rec.KeyAlias,
		KeyPreview:        rec.KeyPreview,
		GroupName:         rec.GroupName,
		Tags:              rec.Tags,
		IsActive:          rec.IsActive,
		Weight:            rec.Weight,
		MonthlyBudgetUSD:  rec.MonthlyBudgetUSD,
		CurrentMonthSpend: rec.CurrentMonthSpend,
		UseCount:          rec.UseCount,
		CreatedAt:         rec.CreatedAt,
		UpdatedAt:         rec.UpdatedAt,
		LastUsedAt:        rec.LastUsedAt,
	}
}

// keyPreview returns "...XXXX" using the last 4 characters of the key.
func keyPreview(key string) string {
	if len(key) <= 4 {
		return "..."
	}
	return "..." + key[len(key)-4:]
}
