package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/llm-router/gateway/internal/gateway/fallback"
	"github.com/llm-router/gateway/internal/gateway/proxy"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
	"github.com/llm-router/gateway/internal/telemetry"
)

// ChatHandler handles POST /v1/chat/completions.
type ChatHandler struct {
	fallbackRouter *fallback.Router
	chains         map[string]fallback.Chain // name → chain, from routing config
	logger         *slog.Logger
}

// NewChatHandler returns a ChatHandler wired to the given fallback router.
func NewChatHandler(fr *fallback.Router, logger *slog.Logger) *ChatHandler {
	return &ChatHandler{
		fallbackRouter: fr,
		chains:         make(map[string]fallback.Chain),
		logger:         logger,
	}
}

// WithChains attaches named fallback chains (loaded from routing config).
func (h *ChatHandler) WithChains(chains []fallback.Chain) *ChatHandler {
	for _, c := range chains {
		h.chains[c.Name] = c
	}
	return h
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

	chain := h.resolveChain(req.Model)
	if len(chain.Targets) == 0 {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("no provider available for model: %s", req.Model),
			"invalid_request_error", "model_not_found")
		return
	}

	// Set primary provider for telemetry (first target in chain).
	telemetry.SetModel(r.Context(), req.Model, chain.Targets[0].Provider)

	if req.Stream {
		telemetry.SetStreaming(r.Context())
		h.handleStream(w, r, &req, body, chain)
		return
	}

	resp, used, err := h.fallbackRouter.Execute(r.Context(), chain, &req, body)
	if err != nil {
		h.logger.Error("chat completion failed",
			"chain", chain.Name,
			"error", err)
		var gwErr *provider.GatewayError
		if errors.As(err, &gwErr) {
			telemetry.SetError(r.Context(), string(gwErr.Code), gwErr.Message)
			writeError(w, gwErr.HTTPStatus, gwErr.Message, string(gwErr.Code), "")
		} else {
			telemetry.SetError(r.Context(), "api_error", err.Error())
			writeError(w, http.StatusBadGateway, err.Error(), "api_error", "")
		}
		return
	}

	// Update telemetry with the actual provider used (may differ from primary after fallback).
	if used.Provider != chain.Targets[0].Provider {
		telemetry.SetModel(r.Context(), req.Model, used.Provider)
	}

	if resp.Usage != nil {
		telemetry.SetTokens(r.Context(), resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	}
	if len(resp.Choices) > 0 {
		telemetry.SetFinishReason(r.Context(), resp.Choices[0].FinishReason)
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleStream proxies a streaming chat completion with fallback support.
func (h *ChatHandler) handleStream(w http.ResponseWriter, r *http.Request, req *types.ChatCompletionRequest, body []byte, chain fallback.Chain) {
	ch, used, err := h.fallbackRouter.ExecuteStream(r.Context(), chain, req, body)
	if err != nil {
		h.logger.Error("stream init failed",
			"chain", chain.Name,
			"error", err)
		var gwErr *provider.GatewayError
		if errors.As(err, &gwErr) {
			telemetry.SetError(r.Context(), string(gwErr.Code), gwErr.Message)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(gwErr.HTTPStatus)
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"error": map[string]string{
					"message": gwErr.Message,
					"type":    string(gwErr.Code),
				},
			})
		} else {
			telemetry.SetError(r.Context(), "stream_error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"error": map[string]string{
					"message": err.Error(),
					"type":    "api_error",
				},
			})
		}
		return
	}

	if used.Provider != chain.Targets[0].Provider {
		telemetry.SetModel(r.Context(), req.Model, used.Provider)
	}

	proxy.StreamChannelToClient(w, r, ch, req.Model, used.Provider, h.logger)
}

// resolveChain determines the fallback chain for the given model string.
// If the model matches a configured chain name, that chain is used.
// Otherwise, a single-target chain is built from the "provider/model" prefix.
func (h *ChatHandler) resolveChain(model string) fallback.Chain {
	// Check named chains first (e.g. "default").
	if c, ok := h.chains[model]; ok {
		return c
	}

	// Parse "provider/model" format.
	parsed := parseModel(model)
	return fallback.SingleTarget(parsed.Provider, parsed.Model)
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
