package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/auth"
	"github.com/llm-router/gateway/internal/cost"
	"github.com/llm-router/gateway/internal/gateway/fallback"
	"github.com/llm-router/gateway/internal/gateway/proxy"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
	"github.com/llm-router/gateway/internal/telemetry"
)

// AdminPlaygroundHandler handles POST /admin/playground.
// Protected by master key auth; no virtual key required.
type AdminPlaygroundHandler struct {
	fr       *fallback.Router
	keyStore auth.Store
	costCalc *cost.Calculator
	logger   *slog.Logger
}

func NewAdminPlaygroundHandler(fr *fallback.Router, keyStore auth.Store, costCalc *cost.Calculator, logger *slog.Logger) *AdminPlaygroundHandler {
	return &AdminPlaygroundHandler{fr: fr, keyStore: keyStore, costCalc: costCalc, logger: logger}
}

// playgroundRequest wraps ChatCompletionRequest with an optional key_id for
// telemetry association (logging only — no budget enforcement for admin calls).
type playgroundRequest struct {
	KeyID string `json:"key_id,omitempty"`
	types.ChatCompletionRequest
}

func (h *AdminPlaygroundHandler) Chat(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body", "invalid_request_error", "")
		return
	}

	var preq playgroundRequest
	if err := json.Unmarshal(body, &preq); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error", "")
		return
	}
	req := &preq.ChatCompletionRequest
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required", "invalid_request_error", "")
		return
	}
	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages is required", "invalid_request_error", "")
		return
	}

	// Associate request with a virtual key for telemetry (logging only; no budget enforcement).
	if preq.KeyID != "" {
		if keyID, err := uuid.Parse(preq.KeyID); err == nil && h.keyStore != nil {
			if key, err := h.keyStore.GetByID(r.Context(), keyID); err == nil {
				telemetry.SetVirtualKeyInfo(r.Context(), &key.ID, key.UserID, key.TeamID, key.OrgID)
			}
		}
	}

	chain := playgroundChain(req.Model)
	telemetry.SetModel(r.Context(), req.Model, chain.Targets[0].Provider)

	// Re-marshal just the ChatCompletionRequest for the upstream provider.
	upstream, _ := json.Marshal(req)

	if req.Stream {
		telemetry.SetStreaming(r.Context())
		ch, used, err := h.fr.ExecuteStream(r.Context(), chain, req, upstream)
		if err != nil {
			h.logger.Error("playground stream failed", "error", err)
			var gwErr *provider.GatewayError
			if errors.As(err, &gwErr) {
				telemetry.SetError(r.Context(), string(gwErr.Code), gwErr.Message)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(gwErr.HTTPStatus)
				json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": gwErr.Message, "type": string(gwErr.Code)}}) //nolint:errcheck
			} else {
				telemetry.SetError(r.Context(), "api_error", err.Error())
				writeError(w, http.StatusBadGateway, err.Error(), "api_error", "")
			}
			return
		}
		if used.Provider != chain.Targets[0].Provider {
			telemetry.SetModel(r.Context(), req.Model, used.Provider)
		}
		proxy.StreamChannelToClient(w, r, ch, req.Model, used.Provider, h.logger)
		h.recordCost(r)
		return
	}

	resp, used, err := h.fr.Execute(r.Context(), chain, req, upstream)
	if err != nil {
		h.logger.Error("playground chat failed", "error", err)
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
	if used.Provider != chain.Targets[0].Provider {
		telemetry.SetModel(r.Context(), req.Model, used.Provider)
	}
	if resp.Usage != nil {
		telemetry.SetTokens(r.Context(), resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	}
	if len(resp.Choices) > 0 {
		telemetry.SetFinishReason(r.Context(), resp.Choices[0].FinishReason)
	}
	h.recordCost(r)
	writeJSON(w, http.StatusOK, resp)
}

// recordCost calculates and sets CostUSD in the request log context.
// BudgetCheck middleware is not applied to admin routes, so playground must do this itself.
func (h *AdminPlaygroundHandler) recordCost(r *http.Request) {
	if h.costCalc == nil {
		return
	}
	lc := telemetry.GetRequestLogContext(r.Context())
	if lc == nil {
		return
	}
	lc.CostUSD = h.costCalc.Calculate(lc.Model, lc.PromptTokens, lc.CompletionTokens)
}

// playgroundChain builds a single-target fallback chain from "provider/model".
func playgroundChain(model string) fallback.Chain {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) == 2 && parts[0] != "" {
		return fallback.SingleTarget(parts[0], parts[1])
	}
	return fallback.SingleTarget("openai", model)
}
