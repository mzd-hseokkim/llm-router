package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/llm-router/gateway/internal/abtest"
	"github.com/llm-router/gateway/internal/gateway/middleware"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

// AdminABTestsHandler serves /admin/ab-tests/* endpoints.
type AdminABTestsHandler struct {
	store  *pgstore.ABTestStore
	abMw   *middleware.ABTestMiddleware
}

// NewAdminABTestsHandler returns a new handler.
func NewAdminABTestsHandler(store *pgstore.ABTestStore, abMw *middleware.ABTestMiddleware) *AdminABTestsHandler {
	return &AdminABTestsHandler{store: store, abMw: abMw}
}

// Create handles POST /admin/ab-tests.
func (h *AdminABTestsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name            string               `json:"name"`
		TrafficSplit    []abtest.TrafficSplit `json:"traffic_split"`
		Target          abtest.Target        `json:"target"`
		SuccessMetrics  []string             `json:"success_metrics"`
		MinSamples      int                  `json:"min_samples"`
		ConfidenceLevel float64              `json:"confidence_level"`
		StartAt         *time.Time           `json:"start_at"`
		EndAt           *time.Time           `json:"end_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error", "")
		return
	}
	if body.Name == "" || len(body.TrafficSplit) < 2 {
		writeError(w, http.StatusBadRequest, "name and at least 2 traffic_split entries are required", "invalid_request_error", "")
		return
	}
	if body.MinSamples <= 0 {
		body.MinSamples = 1000
	}
	if body.ConfidenceLevel <= 0 {
		body.ConfidenceLevel = 0.95
	}
	if body.Target.SampleRate <= 0 {
		body.Target.SampleRate = 1.0
	}

	exp := &abtest.Experiment{
		Name:            body.Name,
		Status:          abtest.StatusDraft,
		TrafficSplit:    body.TrafficSplit,
		Target:          body.Target,
		SuccessMetrics:  body.SuccessMetrics,
		MinSamples:      body.MinSamples,
		ConfidenceLevel: body.ConfidenceLevel,
		StartAt:         body.StartAt,
		EndAt:           body.EndAt,
	}
	if err := h.store.Create(r.Context(), exp); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	writeJSON(w, http.StatusCreated, exp)
}

// List handles GET /admin/ab-tests.
func (h *AdminABTestsHandler) List(w http.ResponseWriter, r *http.Request) {
	tests, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tests})
}

// Get handles GET /admin/ab-tests/{id}.
func (h *AdminABTestsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	exp, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "experiment not found", "api_error", "")
		return
	}
	writeJSON(w, http.StatusOK, exp)
}

// Results handles GET /admin/ab-tests/{id}/results — returns statistical analysis.
func (h *AdminABTestsHandler) Results(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	exp, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "experiment not found", "api_error", "")
		return
	}

	statsMap := make(map[string]*abtest.VariantStats)
	for _, split := range exp.TrafficSplit {
		s, err := h.store.GetVariantStats(r.Context(), id, split.Variant)
		if err == nil {
			statsMap[split.Variant] = s
		}
	}

	result := abtest.Analyze(exp, statsMap)
	writeJSON(w, http.StatusOK, result)
}

// Pause handles POST /admin/ab-tests/{id}/pause.
func (h *AdminABTestsHandler) Pause(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, string(abtest.StatusPaused))
}

// Stop handles POST /admin/ab-tests/{id}/stop.
func (h *AdminABTestsHandler) Stop(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, string(abtest.StatusStopped))
}

// Start handles POST /admin/ab-tests/{id}/start — transitions draft/paused → running.
func (h *AdminABTestsHandler) Start(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, string(abtest.StatusRunning))
}

// Promote handles POST /admin/ab-tests/{id}/promote — marks winner and stops experiment.
func (h *AdminABTestsHandler) Promote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Winner string `json:"winner"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Winner == "" {
		writeError(w, http.StatusBadRequest, "winner variant is required", "invalid_request_error", "")
		return
	}
	if err := h.store.UpdateStatus(r.Context(), id, string(abtest.StatusCompleted), body.Winner); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	if h.abMw != nil {
		_ = h.abMw.Reload(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "completed", "winner": body.Winner})
}

func (h *AdminABTestsHandler) transition(w http.ResponseWriter, r *http.Request, status string) {
	id := chi.URLParam(r, "id")
	if err := h.store.UpdateStatus(r.Context(), id, status, ""); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	if h.abMw != nil {
		_ = h.abMw.Reload(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}
