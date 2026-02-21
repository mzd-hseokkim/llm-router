package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/llm-router/gateway/internal/guardrail"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

// AdminGuardrailsHandler handles CRUD for guardrail policies.
type AdminGuardrailsHandler struct {
	store         guardrail.PolicyStore
	manager       *guardrail.Manager
	logger        *slog.Logger
	buildPipeline func(recs []*guardrail.PolicyRecord) *guardrail.Pipeline
}

// NewAdminGuardrailsHandler creates a new handler.
// buildPipeline is called after any update to rebuild and hot-reload the pipeline.
func NewAdminGuardrailsHandler(
	store guardrail.PolicyStore,
	manager *guardrail.Manager,
	logger *slog.Logger,
	buildPipeline func(recs []*guardrail.PolicyRecord) *guardrail.Pipeline,
) *AdminGuardrailsHandler {
	return &AdminGuardrailsHandler{
		store:         store,
		manager:       manager,
		logger:        logger,
		buildPipeline: buildPipeline,
	}
}

// --- Request / Response types ---

type guardrailPolicyResponse struct {
	ID            string          `json:"id"`
	GuardrailType string          `json:"guardrail_type"`
	IsEnabled     bool            `json:"is_enabled"`
	Action        string          `json:"action"`
	Engine        string          `json:"engine,omitempty"`
	ConfigJSON    json.RawMessage `json:"config_json"`
	SortOrder     int             `json:"sort_order"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type updateGuardrailRequest struct {
	IsEnabled  *bool           `json:"is_enabled,omitempty"`
	Action     *string         `json:"action,omitempty"`
	Engine     *string         `json:"engine,omitempty"`
	ConfigJSON json.RawMessage `json:"config_json,omitempty"`
	SortOrder  *int            `json:"sort_order,omitempty"`
}

type updateAllGuardrailsRequest struct {
	Policies []updateAllPolicyEntry `json:"policies"`
}

type updateAllPolicyEntry struct {
	GuardrailType string          `json:"guardrail_type"`
	IsEnabled     *bool           `json:"is_enabled,omitempty"`
	Action        *string         `json:"action,omitempty"`
	Engine        *string         `json:"engine,omitempty"`
	ConfigJSON    json.RawMessage `json:"config_json,omitempty"`
	SortOrder     *int            `json:"sort_order,omitempty"`
}

// --- Handlers ---

// List handles GET /admin/guardrails.
func (h *AdminGuardrailsHandler) List(w http.ResponseWriter, r *http.Request) {
	recs, err := h.store.List(r.Context())
	if err != nil {
		h.logger.Error("failed to list guardrail policies", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list guardrail policies", "internal_error", "")
		return
	}

	resp := make([]guardrailPolicyResponse, 0, len(recs))
	for _, rec := range recs {
		resp = append(resp, toGuardrailResponse(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": resp})
}

// Get handles GET /admin/guardrails/{type}.
func (h *AdminGuardrailsHandler) Get(w http.ResponseWriter, r *http.Request) {
	guardrailType := chi.URLParam(r, "type")

	rec, err := h.store.GetByType(r.Context(), guardrailType)
	if err != nil {
		if errors.Is(err, pgstore.ErrGuardrailPolicyNotFound) {
			writeError(w, http.StatusNotFound, "guardrail policy not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to get guardrail policy", "type", guardrailType, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get guardrail policy", "internal_error", "")
		return
	}

	writeJSON(w, http.StatusOK, toGuardrailResponse(rec))
}

// Update handles PUT /admin/guardrails/{type}.
func (h *AdminGuardrailsHandler) Update(w http.ResponseWriter, r *http.Request) {
	guardrailType := chi.URLParam(r, "type")

	rec, err := h.store.GetByType(r.Context(), guardrailType)
	if err != nil {
		if errors.Is(err, pgstore.ErrGuardrailPolicyNotFound) {
			writeError(w, http.StatusNotFound, "guardrail policy not found", "invalid_request_error", "not_found")
			return
		}
		h.logger.Error("failed to get guardrail policy", "type", guardrailType, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get guardrail policy", "internal_error", "")
		return
	}

	var req updateGuardrailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error", "")
		return
	}

	if req.IsEnabled != nil {
		rec.IsEnabled = *req.IsEnabled
	}
	if req.Action != nil {
		rec.Action = *req.Action
	}
	if req.Engine != nil {
		rec.Engine = *req.Engine
	}
	if req.ConfigJSON != nil {
		rec.ConfigJSON = []byte(req.ConfigJSON)
	}
	if req.SortOrder != nil {
		rec.SortOrder = *req.SortOrder
	}

	if err := h.store.Upsert(r.Context(), rec); err != nil {
		h.logger.Error("failed to update guardrail policy", "type", guardrailType, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update guardrail policy", "internal_error", "")
		return
	}

	h.rebuildPipeline(r)
	writeJSON(w, http.StatusOK, toGuardrailResponse(rec))
}

// UpdateAll handles PUT /admin/guardrails — bulk update all policies atomically.
func (h *AdminGuardrailsHandler) UpdateAll(w http.ResponseWriter, r *http.Request) {
	var req updateAllGuardrailsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error", "")
		return
	}

	// 1. Fetch all existing records (1 DB call).
	existing, err := h.store.List(r.Context())
	if err != nil {
		h.logger.Error("failed to list guardrail policies", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update policies", "internal_error", "")
		return
	}
	byType := make(map[string]*guardrail.PolicyRecord, len(existing))
	for _, rec := range existing {
		byType[rec.GuardrailType] = rec
	}

	// 2. Apply patches to the in-memory records.
	toUpsert := make([]*guardrail.PolicyRecord, 0, len(req.Policies))
	for _, entry := range req.Policies {
		if entry.GuardrailType == "" {
			continue
		}
		rec, ok := byType[entry.GuardrailType]
		if !ok {
			writeError(w, http.StatusNotFound, "guardrail policy not found: "+entry.GuardrailType, "invalid_request_error", "not_found")
			return
		}
		if entry.IsEnabled != nil {
			rec.IsEnabled = *entry.IsEnabled
		}
		if entry.Action != nil {
			rec.Action = *entry.Action
		}
		if entry.Engine != nil {
			rec.Engine = *entry.Engine
		}
		if entry.ConfigJSON != nil {
			rec.ConfigJSON = []byte(entry.ConfigJSON)
		}
		if entry.SortOrder != nil {
			rec.SortOrder = *entry.SortOrder
		}
		toUpsert = append(toUpsert, rec)
	}

	// 3. Persist all changes in a single transaction (1 DB call).
	if err := h.store.UpsertAll(r.Context(), toUpsert); err != nil {
		h.logger.Error("failed to upsert guardrail policies", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update policies", "internal_error", "")
		return
	}

	// 4. Reload fresh records, rebuild pipeline, return response (1 DB call).
	recs, err := h.store.List(r.Context())
	if err != nil {
		h.logger.Error("failed to list guardrail policies after update", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list guardrail policies", "internal_error", "")
		return
	}
	h.manager.SetPipeline(h.buildPipeline(recs))

	resp := make([]guardrailPolicyResponse, 0, len(recs))
	for _, rec := range recs {
		resp = append(resp, toGuardrailResponse(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": resp})
}

// rebuildPipeline reloads the active pipeline from DB after a policy change.
func (h *AdminGuardrailsHandler) rebuildPipeline(r *http.Request) {
	recs, err := h.store.List(r.Context())
	if err != nil {
		h.logger.Error("guardrails: failed to reload policies after update", "error", err)
		return
	}
	p := h.buildPipeline(recs)
	h.manager.SetPipeline(p)
}

// --- converter ---

func toGuardrailResponse(rec *guardrail.PolicyRecord) guardrailPolicyResponse {
	cfg := json.RawMessage(rec.ConfigJSON)
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	return guardrailPolicyResponse{
		ID:            rec.ID,
		GuardrailType: rec.GuardrailType,
		IsEnabled:     rec.IsEnabled,
		Action:        rec.Action,
		Engine:        rec.Engine,
		ConfigJSON:    cfg,
		SortOrder:     rec.SortOrder,
		CreatedAt:     rec.CreatedAt,
		UpdatedAt:     rec.UpdatedAt,
	}
}
