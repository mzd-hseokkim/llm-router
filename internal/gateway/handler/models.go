package handler

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

// ModelsHandler handles GET /v1/models and GET /v1/models/*.
type ModelsHandler struct {
	registry *provider.Registry
	logger   *slog.Logger
}

// NewModelsHandler returns a ModelsHandler wired to the given registry.
func NewModelsHandler(registry *provider.Registry, logger *slog.Logger) *ModelsHandler {
	return &ModelsHandler{registry: registry, logger: logger}
}

// List returns all models registered across all providers.
func (h *ModelsHandler) List(w http.ResponseWriter, r *http.Request) {
	models := h.registry.AllModels()
	if models == nil {
		models = []types.ModelInfo{}
	}
	writeJSON(w, http.StatusOK, types.ModelListResponse{
		Object: "list",
		Data:   models,
	})
}

// Get returns a single model by its ID.
// The model ID may contain a slash (e.g. "anthropic/claude-sonnet-4-20250514"),
// so the route uses a wildcard: GET /v1/models/*.
func (h *ModelsHandler) Get(w http.ResponseWriter, r *http.Request) {
	modelID := chi.URLParam(r, "*")
	if modelID == "" {
		writeError(w, http.StatusBadRequest, "model ID is required", "invalid_request_error", "")
		return
	}

	for _, m := range h.registry.AllModels() {
		if m.ID == modelID {
			writeJSON(w, http.StatusOK, m)
			return
		}
	}

	writeError(w, http.StatusNotFound,
		fmt.Sprintf("The model '%s' does not exist", modelID),
		"invalid_request_error", "model_not_found")
}
