package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

// EmbeddingsHandler handles POST /v1/embeddings.
type EmbeddingsHandler struct {
	registry *provider.Registry
	logger   *slog.Logger
}

// NewEmbeddingsHandler returns an EmbeddingsHandler wired to the given registry.
func NewEmbeddingsHandler(registry *provider.Registry, logger *slog.Logger) *EmbeddingsHandler {
	return &EmbeddingsHandler{registry: registry, logger: logger}
}

// Handle processes an embeddings request.
func (h *EmbeddingsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body", "invalid_request_error", "")
		return
	}

	var req types.EmbeddingRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error", "")
		return
	}

	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required", "invalid_request_error", "")
		return
	}
	if len(req.Input) == 0 {
		writeError(w, http.StatusBadRequest, "input is required", "invalid_request_error", "")
		return
	}

	parsed := parseModel(req.Model)
	if _, ok := h.registry.Get(parsed.Provider); !ok {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("Invalid model: %s", req.Model),
			"invalid_request_error", "model_not_found")
		return
	}

	// TODO: delegate to provider embedding (task 03)
	writeJSON(w, http.StatusOK, types.EmbeddingResponse{
		Object: "list",
		Data:   []types.Embedding{},
		Model:  req.Model,
	})
}
