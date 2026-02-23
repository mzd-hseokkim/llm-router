package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/llm-router/gateway/internal/gateway/fallback"
	"github.com/llm-router/gateway/internal/gateway/proxy"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
	"github.com/llm-router/gateway/internal/telemetry"
)

// CompletionsHandler handles POST /v1/completions (legacy text completions).
type CompletionsHandler struct {
	fallbackRouter *fallback.Router
	logger         *slog.Logger
}

// NewCompletionsHandler returns a CompletionsHandler wired to the given fallback router.
func NewCompletionsHandler(fr *fallback.Router, logger *slog.Logger) *CompletionsHandler {
	return &CompletionsHandler{fallbackRouter: fr, logger: logger}
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

	msgs, err := promptToMessages(req.Prompt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "")
		return
	}

	parsed := parseModel(req.Model)
	chain := fallback.SingleTarget(parsed.Provider, parsed.Model)

	chatReq := &types.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    msgs,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream,
		User:        req.User,
	}

	telemetry.SetModel(r.Context(), req.Model, parsed.Provider)

	if req.Stream {
		telemetry.SetStreaming(r.Context())
		ch, used, err := h.fallbackRouter.ExecuteStream(r.Context(), chain, chatReq, body)
		if err != nil {
			h.logger.Error("completions stream init failed",
				"chain", chain.Name,
				"error", err)
			var gwErr *provider.GatewayError
			if errors.As(err, &gwErr) {
				telemetry.SetError(r.Context(), string(gwErr.Code), gwErr.Message)
				writeError(w, gwErr.HTTPStatus, gwErr.Message, string(gwErr.Code), "")
			} else {
				telemetry.SetError(r.Context(), "stream_error", err.Error())
				writeError(w, http.StatusBadGateway, err.Error(), "api_error", "")
			}
			return
		}
		telemetry.SetModel(r.Context(), req.Model, used.Provider)
		proxy.StreamTextChannelToClient(w, r, ch, req.Model, used.Provider, h.logger)
		return
	}

	resp, used, err := h.fallbackRouter.Execute(r.Context(), chain, chatReq, body)
	if err != nil {
		h.logger.Error("completions failed",
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

	if used.Provider != parsed.Provider {
		telemetry.SetModel(r.Context(), req.Model, used.Provider)
	}

	if resp.Usage != nil {
		telemetry.SetTokens(r.Context(), resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	}

	cmplResp := chatToCmplResponse(resp, req.Model)
	if len(cmplResp.Choices) > 0 {
		telemetry.SetFinishReason(r.Context(), cmplResp.Choices[0].FinishReason)
	}

	writeJSON(w, http.StatusOK, cmplResp)
}

// promptToMessages converts a CompletionRequest prompt (string or []string)
// to a single-element ChatCompletionRequest messages slice with role "user".
// Only the first string is used when the prompt is an array.
func promptToMessages(raw json.RawMessage) ([]types.Message, error) {
	// Try string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return nil, fmt.Errorf("prompt must not be empty")
		}
		return []types.Message{{Role: "user", Content: s}}, nil
	}

	// Try []string.
	var ss []string
	if err := json.Unmarshal(raw, &ss); err == nil {
		if len(ss) == 0 || ss[0] == "" {
			return nil, fmt.Errorf("prompt must not be empty")
		}
		return []types.Message{{Role: "user", Content: ss[0]}}, nil
	}

	return nil, fmt.Errorf("prompt must be a string or array of strings")
}

// chatToCmplResponse converts a ChatCompletionResponse to a CompletionResponse.
func chatToCmplResponse(chat *types.ChatCompletionResponse, model string) types.CompletionResponse {
	choices := make([]types.CompletionChoice, len(chat.Choices))
	for i, c := range chat.Choices {
		choices[i] = types.CompletionChoice{
			Index:        c.Index,
			Text:         c.Message.Content,
			FinishReason: c.FinishReason,
		}
	}
	return types.CompletionResponse{
		ID:      chat.ID,
		Object:  "text_completion",
		Created: chat.Created,
		Model:   model,
		Choices: choices,
		Usage:   chat.Usage,
	}
}
