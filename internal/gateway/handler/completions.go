package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

// CompletionsHandler handles POST /v1/completions (legacy text completions).
type CompletionsHandler struct {
	registry *provider.Registry
	logger   *slog.Logger
}

// NewCompletionsHandler returns a CompletionsHandler wired to the given registry.
func NewCompletionsHandler(registry *provider.Registry, logger *slog.Logger) *CompletionsHandler {
	return &CompletionsHandler{registry: registry, logger: logger}
}

// Handle processes a legacy text completions request.
func (h *CompletionsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body", "invalid_request_error", "")
		return
	}

	var req types.CompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error", "")
		return
	}

	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required", "invalid_request_error", "")
		return
	}
	if len(req.Prompt) == 0 {
		writeError(w, http.StatusBadRequest, "prompt is required", "invalid_request_error", "")
		return
	}
	if req.MaxTokens != nil && *req.MaxTokens <= 0 {
		writeError(w, http.StatusBadRequest, "max_tokens must be a positive integer", "invalid_request_error", "")
		return
	}

	parsed := parseModel(req.Model)
	if _, ok := h.registry.Get(parsed.Provider); !ok {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("Invalid model: %s", req.Model),
			"invalid_request_error", "model_not_found")
		return
	}

	// TODO: delegate to provider text completion (task 03)
	writeJSON(w, http.StatusOK, types.CompletionResponse{
		ID:      generateID("cmpl"),
		Object:  "text_completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []types.CompletionChoice{{
			Index:        0,
			Text:         "",
			FinishReason: "stop",
		}},
	})
}
