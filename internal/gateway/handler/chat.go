package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/llm-router/gateway/internal/gateway/proxy"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
	"github.com/llm-router/gateway/internal/telemetry"
)

// ChatHandler handles POST /v1/chat/completions.
type ChatHandler struct {
	registry *provider.Registry
	logger   *slog.Logger
}

// NewChatHandler returns a ChatHandler wired to the given provider registry.
func NewChatHandler(registry *provider.Registry, logger *slog.Logger) *ChatHandler {
	return &ChatHandler{registry: registry, logger: logger}
}

// Handle processes a chat completions request.
func (h *ChatHandler) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MB limit
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body", "invalid_request_error", "")
		return
	}

	var req types.ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error", "")
		return
	}

	if err := validateChatRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}

	parsed := parseModel(req.Model)
	p, ok := h.registry.Get(parsed.Provider)
	if !ok {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("Invalid model: %s", req.Model),
			"invalid_request_error", "model_not_found")
		return
	}

	telemetry.SetModel(r.Context(), req.Model, parsed.Provider)

	if req.Stream {
		telemetry.SetStreaming(r.Context())
		h.handleStream(w, r, &req, body, p, parsed)
		return
	}

	resp, err := p.ChatCompletion(r.Context(), parsed.Model, &req, body)
	if err != nil {
		h.logger.Error("provider chat completion failed",
			"provider", parsed.Provider,
			"model", parsed.Model,
			"error", err)
		telemetry.SetError(r.Context(), "api_error", err.Error())
		writeError(w, http.StatusBadGateway, err.Error(), "api_error", "")
		return
	}

	if resp.Usage != nil {
		telemetry.SetTokens(r.Context(), resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	}
	if len(resp.Choices) > 0 {
		telemetry.SetFinishReason(r.Context(), resp.Choices[0].FinishReason)
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleStream proxies a streaming chat completion to the client.
func (h *ChatHandler) handleStream(w http.ResponseWriter, r *http.Request, req *types.ChatCompletionRequest, body []byte, p provider.Provider, parsed types.ParsedModel) {
	proxy.StreamToClient(w, r, p, parsed, req, body, h.logger)
}

// validateChatRequest checks required fields and value ranges.
func validateChatRequest(req *types.ChatCompletionRequest) error {
	if req.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(req.Messages) == 0 {
		return fmt.Errorf("messages is required and must not be empty")
	}
	if req.Temperature != nil && (*req.Temperature < 0 || *req.Temperature > 2) {
		return fmt.Errorf("temperature must be between 0.0 and 2.0, got %g", *req.Temperature)
	}
	if req.MaxTokens != nil && *req.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be a positive integer")
	}
	return nil
}

// parseModel splits "provider/model" into its parts.
// If no slash is present, or the provider part is empty, defaults to "openai".
func parseModel(model string) types.ParsedModel {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) == 2 && parts[0] != "" {
		return types.ParsedModel{Provider: parts[0], Model: parts[1]}
	}
	return types.ParsedModel{Provider: "openai", Model: model}
}
