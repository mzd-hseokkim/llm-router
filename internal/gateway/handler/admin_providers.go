package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/llm-router/gateway/internal/cost"
	"github.com/llm-router/gateway/internal/crypto"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

// AdminProvidersHandler handles CRUD for providers and their models.
type AdminProvidersHandler struct {
	providerStore provider.ProviderStore
	modelStore    provider.ModelStore
	registry      *provider.Registry
	costCalc      *cost.Calculator
	cipher        *crypto.Cipher
	km            *provider.KeyManager
	pool          *pgxpool.Pool
	logger        *slog.Logger
}

// NewAdminProvidersHandler creates a new handler.
func NewAdminProvidersHandler(
	providerStore provider.ProviderStore,
	modelStore provider.ModelStore,
	registry *provider.Registry,
	costCalc *cost.Calculator,
	cipher *crypto.Cipher,
	km *provider.KeyManager,
	pool *pgxpool.Pool,
	logger *slog.Logger,
) *AdminProvidersHandler {
	return &AdminProvidersHandler{
		providerStore: providerStore,
		modelStore:    modelStore,
		registry:      registry,
		costCalc:      costCalc,
		cipher:        cipher,
		km:            km,
		pool:          pool,
		logger:        logger,
	}
}

// --- Request / Response types ---

type createProviderRequest struct {
	Name        string          `json:"name"`
	AdapterType string          `json:"adapter_type"`
	DisplayName string          `json:"display_name"`
	BaseURL     string          `json:"base_url,omitempty"`
	IsEnabled   *bool           `json:"is_enabled,omitempty"`
	ConfigJSON  json.RawMessage `json:"config_json,omitempty"`
	SortOrder   int             `json:"sort_order"`
	APIKey      string          `json:"api_key,omitempty"` // optional: creates a default key atomically
}

type updateProviderRequest struct {
	DisplayName *string         `json:"display_name,omitempty"`
	BaseURL     *string         `json:"base_url,omitempty"`
	IsEnabled   *bool           `json:"is_enabled,omitempty"`
	ConfigJSON  json.RawMessage `json:"config_json,omitempty"`
	SortOrder   *int            `json:"sort_order,omitempty"`
}

type providerResponse struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	AdapterType string          `json:"adapter_type"`
	DisplayName string          `json:"display_name"`
	BaseURL     string          `json:"base_url,omitempty"`
	IsEnabled   bool            `json:"is_enabled"`
	ConfigJSON  json.RawMessage `json:"config_json,omitempty"`
	SortOrder   int             `json:"sort_order"`
	ModelCount  int             `json:"model_count,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type createModelRequest struct {
	ModelID                 string   `json:"model_id"`
	ModelName               string   `json:"model_name"`
	DisplayName             string   `json:"display_name,omitempty"`
	IsEnabled               *bool    `json:"is_enabled,omitempty"`
	InputPerMillionTokens   float64  `json:"input_per_million_tokens"`
	OutputPerMillionTokens  float64  `json:"output_per_million_tokens"`
	ContextWindow           *int     `json:"context_window,omitempty"`
	MaxOutputTokens         *int     `json:"max_output_tokens,omitempty"`
	SupportsStreaming        *bool    `json:"supports_streaming,omitempty"`
	SupportsTools           *bool    `json:"supports_tools,omitempty"`
	SupportsVision          *bool    `json:"supports_vision,omitempty"`
	Tags                    []string `json:"tags,omitempty"`
	SortOrder               int      `json:"sort_order"`
}

type updateModelRequest struct {
	ModelName               *string  `json:"model_name,omitempty"`
	DisplayName             *string  `json:"display_name,omitempty"`
	IsEnabled               *bool    `json:"is_enabled,omitempty"`
	InputPerMillionTokens   *float64 `json:"input_per_million_tokens,omitempty"`
	OutputPerMillionTokens  *float64 `json:"output_per_million_tokens,omitempty"`
	ContextWindow           *int     `json:"context_window,omitempty"`
	MaxOutputTokens         *int     `json:"max_output_tokens,omitempty"`
	SupportsStreaming        *bool    `json:"supports_streaming,omitempty"`
	SupportsTools           *bool    `json:"supports_tools,omitempty"`
	SupportsVision          *bool    `json:"supports_vision,omitempty"`
	Tags                    []string `json:"tags,omitempty"`
	SortOrder               *int     `json:"sort_order,omitempty"`
}

type modelResponse struct {
	ID                      uuid.UUID `json:"id"`
	ProviderID              uuid.UUID `json:"provider_id"`
	ModelID                 string    `json:"model_id"`
	ModelName               string    `json:"model_name"`
	DisplayName             string    `json:"display_name,omitempty"`
	IsEnabled               bool      `json:"is_enabled"`
	InputPerMillionTokens   float64   `json:"input_per_million_tokens"`
	OutputPerMillionTokens  float64   `json:"output_per_million_tokens"`
	ContextWindow           *int      `json:"context_window,omitempty"`
	MaxOutputTokens         *int      `json:"max_output_tokens,omitempty"`
	SupportsStreaming        bool      `json:"supports_streaming"`
	SupportsTools           bool      `json:"supports_tools"`
	SupportsVision          bool      `json:"supports_vision"`
	Tags                    []string  `json:"tags,omitempty"`
	SortOrder               int       `json:"sort_order"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// --- Provider handlers ---

// ListProviders handles GET /admin/providers.
func (h *AdminProvidersHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	recs, err := h.providerStore.List(r.Context())
	if err != nil {
		h.logger.Error("failed to list providers", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list providers", "internal_error", "")
		return
	}

	resp := make([]providerResponse, 0, len(recs))
	for _, rec := range recs {
		models, _ := h.modelStore.ListByProvider(r.Context(), rec.ID)
		pr := toProviderResponse(rec)
		pr.ModelCount = len(models)
		resp = append(resp, pr)
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": resp})
}

// CreateProvider handles POST /admin/providers.
// If api_key is provided, the provider and its default key are inserted atomically.
func (h *AdminProvidersHandler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	var req createProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error", "")
		return
	}
	if req.Name == "" || req.AdapterType == "" {
		writeError(w, http.StatusBadRequest, "name and adapter_type are required", "invalid_request_error", "")
		return
	}

	isEnabled := true
	if req.IsEnabled != nil {
		isEnabled = *req.IsEnabled
	}

	rec := &provider.ProviderRecord{
		Name:        req.Name,
		AdapterType: req.AdapterType,
		DisplayName: req.DisplayName,
		BaseURL:     req.BaseURL,
		IsEnabled:   isEnabled,
		ConfigJSON:  []byte(req.ConfigJSON),
		SortOrder:   req.SortOrder,
	}

	// No API key: simple single-table insert.
	if req.APIKey == "" {
		if err := h.providerStore.Create(r.Context(), rec); err != nil {
			h.logger.Error("failed to create provider", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to create provider", "internal_error", "")
			return
		}
		writeJSON(w, http.StatusCreated, toProviderResponse(rec))
		return
	}

	// API key provided: encrypt then insert both records in one transaction.
	if h.cipher == nil {
		writeError(w, http.StatusBadRequest, "encryption not configured; cannot store API key", "invalid_request_error", "")
		return
	}

	encryptedKey, err := h.cipher.Encrypt([]byte(req.APIKey))
	if err != nil {
		h.logger.Error("failed to encrypt provider key", "error", err)
		writeError(w, http.StatusInternalServerError, "encryption failed", "internal_error", "")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.logger.Error("failed to begin transaction", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create provider", "internal_error", "")
		return
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck

	const providerSQL = `
		INSERT INTO providers (name, adapter_type, display_name, base_url, is_enabled, config_json, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	var provID pgtype.UUID
	var provCreatedAt, provUpdatedAt pgtype.Timestamptz
	err = tx.QueryRow(r.Context(), providerSQL,
		rec.Name, rec.AdapterType, rec.DisplayName,
		nullableStrH(rec.BaseURL), rec.IsEnabled,
		nullableBH(rec.ConfigJSON), rec.SortOrder,
	).Scan(&provID, &provCreatedAt, &provUpdatedAt)
	if err != nil {
		h.logger.Error("failed to insert provider", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create provider", "internal_error", "")
		return
	}
	rec.ID = uuid.UUID(provID.Bytes)
	if provCreatedAt.Valid {
		rec.CreatedAt = provCreatedAt.Time
	}
	if provUpdatedAt.Valid {
		rec.UpdatedAt = provUpdatedAt.Time
	}

	const keySQL = `
		INSERT INTO provider_keys (provider, key_alias, encrypted_key, key_preview, is_active, weight, monthly_budget_usd)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err = tx.Exec(r.Context(), keySQL,
		req.Name, "default", encryptedKey, keyPreview(req.APIKey), true, 100, nil,
	)
	if err != nil {
		h.logger.Error("failed to insert provider key", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create provider key", "internal_error", "")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		h.logger.Error("failed to commit transaction", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create provider", "internal_error", "")
		return
	}

	if h.km != nil {
		h.km.InvalidateCache(req.Name)
	}
	writeJSON(w, http.StatusCreated, toProviderResponse(rec))
}

// GetProvider handles GET /admin/providers/{id}.
func (h *AdminProvidersHandler) GetProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	rec, err := h.providerStore.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgstore.ErrProviderNotFound) {
			writeError(w, http.StatusNotFound, "provider not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to get provider", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get provider", "internal_error", "")
		return
	}

	writeJSON(w, http.StatusOK, toProviderResponse(rec))
}

// UpdateProvider handles PUT /admin/providers/{id}.
func (h *AdminProvidersHandler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	rec, err := h.providerStore.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgstore.ErrProviderNotFound) {
			writeError(w, http.StatusNotFound, "provider not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to get provider", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get provider", "internal_error", "")
		return
	}

	var req updateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error", "")
		return
	}

	if req.DisplayName != nil {
		rec.DisplayName = *req.DisplayName
	}
	if req.BaseURL != nil {
		rec.BaseURL = *req.BaseURL
	}
	if req.IsEnabled != nil {
		rec.IsEnabled = *req.IsEnabled
	}
	if req.ConfigJSON != nil {
		rec.ConfigJSON = []byte(req.ConfigJSON)
	}
	if req.SortOrder != nil {
		rec.SortOrder = *req.SortOrder
	}

	if err := h.providerStore.Update(r.Context(), rec); err != nil {
		h.logger.Error("failed to update provider", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update provider", "internal_error", "")
		return
	}

	writeJSON(w, http.StatusOK, toProviderResponse(rec))
}

// DeleteProvider handles DELETE /admin/providers/{id}.
func (h *AdminProvidersHandler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	if err := h.providerStore.Delete(r.Context(), id); err != nil {
		if errors.Is(err, pgstore.ErrProviderNotFound) {
			writeError(w, http.StatusNotFound, "provider not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to delete provider", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete provider", "internal_error", "")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Model handlers ---

// ListModels handles GET /admin/providers/{id}/models.
func (h *AdminProvidersHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	recs, err := h.modelStore.ListByProvider(r.Context(), providerID)
	if err != nil {
		h.logger.Error("failed to list models", "provider_id", providerID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list models", "internal_error", "")
		return
	}

	resp := make([]modelResponse, 0, len(recs))
	for _, rec := range recs {
		resp = append(resp, toModelResponse(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": resp})
}

// CreateModel handles POST /admin/providers/{id}/models.
func (h *AdminProvidersHandler) CreateModel(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	// Verify provider exists.
	prov, err := h.providerStore.GetByID(r.Context(), providerID)
	if err != nil {
		if errors.Is(err, pgstore.ErrProviderNotFound) {
			writeError(w, http.StatusNotFound, "provider not found", "invalid_request_error", "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get provider", "internal_error", "")
		return
	}

	var req createModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error", "")
		return
	}
	if req.ModelID == "" || req.ModelName == "" {
		writeError(w, http.StatusBadRequest, "model_id and model_name are required", "invalid_request_error", "")
		return
	}

	isEnabled := true
	if req.IsEnabled != nil {
		isEnabled = *req.IsEnabled
	}
	supportsStreaming := true
	if req.SupportsStreaming != nil {
		supportsStreaming = *req.SupportsStreaming
	}
	supportsTools := false
	if req.SupportsTools != nil {
		supportsTools = *req.SupportsTools
	}
	supportsVision := false
	if req.SupportsVision != nil {
		supportsVision = *req.SupportsVision
	}

	rec := &provider.ModelRecord{
		ProviderID:             providerID,
		ModelID:                req.ModelID,
		ModelName:              req.ModelName,
		DisplayName:            req.DisplayName,
		IsEnabled:              isEnabled,
		InputPerMillionTokens:  req.InputPerMillionTokens,
		OutputPerMillionTokens: req.OutputPerMillionTokens,
		ContextWindow:          req.ContextWindow,
		MaxOutputTokens:        req.MaxOutputTokens,
		SupportsStreaming:       supportsStreaming,
		SupportsTools:          supportsTools,
		SupportsVision:         supportsVision,
		Tags:                   req.Tags,
		SortOrder:              req.SortOrder,
	}

	if err := h.modelStore.Create(r.Context(), rec); err != nil {
		h.logger.Error("failed to create model", "provider_id", providerID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create model", "internal_error", "")
		return
	}

	h.syncProviderModels(r, prov)
	writeJSON(w, http.StatusCreated, toModelResponse(rec))
}

// UpdateModel handles PUT /admin/providers/{id}/models/{modelId}.
func (h *AdminProvidersHandler) UpdateModel(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	modelID, ok := parseUUID(w, chi.URLParam(r, "modelId"))
	if !ok {
		return
	}

	rec, err := h.modelStore.GetByID(r.Context(), modelID)
	if err != nil {
		if errors.Is(err, pgstore.ErrModelNotFound) {
			writeError(w, http.StatusNotFound, "model not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to get model", "model_id", modelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get model", "internal_error", "")
		return
	}

	var req updateModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error", "")
		return
	}

	if req.ModelName != nil {
		rec.ModelName = *req.ModelName
	}
	if req.DisplayName != nil {
		rec.DisplayName = *req.DisplayName
	}
	if req.IsEnabled != nil {
		rec.IsEnabled = *req.IsEnabled
	}
	if req.InputPerMillionTokens != nil {
		rec.InputPerMillionTokens = *req.InputPerMillionTokens
	}
	if req.OutputPerMillionTokens != nil {
		rec.OutputPerMillionTokens = *req.OutputPerMillionTokens
	}
	if req.ContextWindow != nil {
		rec.ContextWindow = req.ContextWindow
	}
	if req.MaxOutputTokens != nil {
		rec.MaxOutputTokens = req.MaxOutputTokens
	}
	if req.SupportsStreaming != nil {
		rec.SupportsStreaming = *req.SupportsStreaming
	}
	if req.SupportsTools != nil {
		rec.SupportsTools = *req.SupportsTools
	}
	if req.SupportsVision != nil {
		rec.SupportsVision = *req.SupportsVision
	}
	if req.Tags != nil {
		rec.Tags = req.Tags
	}
	if req.SortOrder != nil {
		rec.SortOrder = *req.SortOrder
	}

	if err := h.modelStore.Update(r.Context(), rec); err != nil {
		h.logger.Error("failed to update model", "model_id", modelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update model", "internal_error", "")
		return
	}

	prov, _ := h.providerStore.GetByID(r.Context(), providerID)
	if prov != nil {
		h.syncProviderModels(r, prov)
	}
	writeJSON(w, http.StatusOK, toModelResponse(rec))
}

// DeleteModel handles DELETE /admin/providers/{id}/models/{modelId}.
func (h *AdminProvidersHandler) DeleteModel(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	modelID, ok := parseUUID(w, chi.URLParam(r, "modelId"))
	if !ok {
		return
	}

	if err := h.modelStore.Delete(r.Context(), modelID); err != nil {
		if errors.Is(err, pgstore.ErrModelNotFound) {
			writeError(w, http.StatusNotFound, "model not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to delete model", "model_id", modelID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete model", "internal_error", "")
		return
	}

	prov, _ := h.providerStore.GetByID(r.Context(), providerID)
	if prov != nil {
		h.syncProviderModels(r, prov)
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- side-effect helpers ---

// syncProviderModels refreshes the in-memory registry and pricing table
// after a model CUD operation.
func (h *AdminProvidersHandler) syncProviderModels(r *http.Request, prov *provider.ProviderRecord) {
	models, err := h.modelStore.ListByProvider(r.Context(), prov.ID)
	if err != nil {
		h.logger.Error("failed to reload models after sync", "provider", prov.Name, "error", err)
		return
	}

	modelInfos := make([]types.ModelInfo, 0, len(models))
	pricing := make(map[string]cost.ModelPricing, len(models))
	for _, m := range models {
		if !m.IsEnabled {
			continue
		}
		modelInfos = append(modelInfos, types.ModelInfo{
			ID:      m.ModelID,
			Object:  "model",
			OwnedBy: prov.Name,
		})
		pricing[m.ModelID] = cost.ModelPricing{
			Provider:               prov.Name,
			InputPerMillionTokens:  m.InputPerMillionTokens,
			OutputPerMillionTokens: m.OutputPerMillionTokens,
		}
	}

	if ok := h.registry.SetProviderModels(prov.Name, modelInfos); !ok {
		h.logger.Warn("syncProviderModels: provider not found in registry (name mismatch?)",
			"db_provider_name", prov.Name)
	}
	h.costCalc.UpdatePricing(pricing)
}

// --- converters ---

func toProviderResponse(rec *provider.ProviderRecord) providerResponse {
	return providerResponse{
		ID:          rec.ID,
		Name:        rec.Name,
		AdapterType: rec.AdapterType,
		DisplayName: rec.DisplayName,
		BaseURL:     rec.BaseURL,
		IsEnabled:   rec.IsEnabled,
		ConfigJSON:  json.RawMessage(rec.ConfigJSON),
		SortOrder:   rec.SortOrder,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
	}
}

// nullableStrH returns nil for empty strings (for nullable DB columns).
func nullableStrH(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullableBH returns nil for empty/nil byte slices (for nullable JSONB columns).
func nullableBH(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func toModelResponse(rec *provider.ModelRecord) modelResponse {
	return modelResponse{
		ID:                     rec.ID,
		ProviderID:             rec.ProviderID,
		ModelID:                rec.ModelID,
		ModelName:              rec.ModelName,
		DisplayName:            rec.DisplayName,
		IsEnabled:              rec.IsEnabled,
		InputPerMillionTokens:  rec.InputPerMillionTokens,
		OutputPerMillionTokens: rec.OutputPerMillionTokens,
		ContextWindow:          rec.ContextWindow,
		MaxOutputTokens:        rec.MaxOutputTokens,
		SupportsStreaming:       rec.SupportsStreaming,
		SupportsTools:          rec.SupportsTools,
		SupportsVision:         rec.SupportsVision,
		Tags:                   rec.Tags,
		SortOrder:              rec.SortOrder,
		CreatedAt:              rec.CreatedAt,
		UpdatedAt:              rec.UpdatedAt,
	}
}
