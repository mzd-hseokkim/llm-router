package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/budget"
)

// AdminBudgetsHandler provides CRUD endpoints for budget management.
type AdminBudgetsHandler struct {
	mgr    *budget.Manager
	store  budget.Store
	logger *slog.Logger
}

// NewAdminBudgetsHandler creates the handler.
func NewAdminBudgetsHandler(mgr *budget.Manager, store budget.Store, logger *slog.Logger) *AdminBudgetsHandler {
	return &AdminBudgetsHandler{mgr: mgr, store: store, logger: logger}
}

type budgetRequest struct {
	EntityType   string   `json:"entity_type"`
	EntityID     string   `json:"entity_id"`
	Period       string   `json:"period"`
	SoftLimitUSD *float64 `json:"soft_limit_usd"`
	HardLimitUSD *float64 `json:"hard_limit_usd"`
}

// Create sets or updates a budget.
// POST /admin/budgets
func (h *AdminBudgetsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req budgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	if err := validateBudgetRequest(req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	entityID, err := uuid.Parse(req.EntityID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid entity_id"})
		return
	}

	now := time.Now()
	start, err1 := budget.PeriodStart(req.Period, now)
	end, err2 := budget.PeriodEnd(req.Period, now)
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid period"})
		return
	}

	b := &budget.Budget{
		EntityType:   req.EntityType,
		EntityID:     entityID,
		Period:       req.Period,
		SoftLimitUSD: req.SoftLimitUSD,
		HardLimitUSD: req.HardLimitUSD,
		PeriodStart:  start,
		PeriodEnd:    end,
	}

	if err := h.store.Upsert(r.Context(), b); err != nil {
		h.logger.Error("budget create", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Evict the in-memory list cache so the next request sees the new budget immediately.
	h.mgr.InvalidateListCache(req.EntityType, entityID)

	writeJSON(w, http.StatusCreated, b)
}

// List returns all budgets for an entity.
// GET /admin/budgets/{entity_type}/{entity_id}
func (h *AdminBudgetsHandler) List(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entity_type")
	entityIDStr := chi.URLParam(r, "entity_id")

	entityID, err := uuid.Parse(entityIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid entity_id"})
		return
	}

	budgets, err := h.store.List(r.Context(), entityType, entityID)
	if err != nil {
		h.logger.Error("budget list", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, budgets)
}

// Reset manually resets a budget's current_spend to zero.
// POST /admin/budgets/{id}/reset
func (h *AdminBudgetsHandler) Reset(w http.ResponseWriter, r *http.Request) {
	// For Phase 2, return 501 — full reset requires knowing entity_type+period.
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": "manual budget reset not yet implemented; budgets reset automatically at period end",
	})
}

func validateBudgetRequest(req budgetRequest) error {
	if req.EntityType == "" {
		return fmt.Errorf("entity_type required")
	}
	if req.EntityID == "" {
		return fmt.Errorf("entity_id required")
	}
	validTypes := map[string]bool{"key": true, "user": true, "team": true, "org": true}
	if !validTypes[req.EntityType] {
		return fmt.Errorf("entity_type must be one of: key, user, team, org")
	}
	validPeriods := map[string]bool{"hourly": true, "daily": true, "weekly": true, "monthly": true, "lifetime": true}
	if !validPeriods[req.Period] {
		return fmt.Errorf("period must be one of: hourly, daily, weekly, monthly, lifetime")
	}
	return nil
}
