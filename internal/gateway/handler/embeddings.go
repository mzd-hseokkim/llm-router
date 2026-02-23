package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	exactcache "github.com/llm-router/gateway/internal/cache/exact"
	"github.com/llm-router/gateway/internal/cost"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
	"github.com/llm-router/gateway/internal/telemetry"
)

// EmbeddingsHandler handles POST /v1/embeddings.
type EmbeddingsHandler struct {
	registry *provider.Registry
	cache    *exactcache.Cache  // optional, nil = no caching
	costCalc *cost.Calculator   // optional, nil = no cost tracking
	logger   *slog.Logger
}

// NewEmbeddingsHandler returns an EmbeddingsHandler wired to the given dependencies.
func NewEmbeddingsHandler(registry *provider.Registry, cache *exactcache.Cache, costCalc *cost.Calculator, logger *slog.Logger) *EmbeddingsHandler {
	return &EmbeddingsHandler{registry: registry, cache: cache, costCalc: costCalc, logger: logger}
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
	prov, ok := h.registry.Get(parsed.Provider)
	if !ok {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("Invalid model: %s", req.Model),
			"invalid_request_error", "model_not_found")
		return
	}

	ep, ok := prov.(provider.EmbeddingProvider)
	if !ok {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("provider %s does not support embeddings", parsed.Provider),
			"invalid_request_error", "")
		return
	}

	// Cache lookup.
	headers := map[string]string{
		"Cache-Control":      r.Header.Get("Cache-Control"),
		"X-Gateway-No-Cache": r.Header.Get("X-Gateway-No-Cache"),
	}
	var cacheKey string
	if h.cache != nil && exactcache.IsEmbeddingCacheable(headers) {
		cacheKey = exactcache.BuildEmbeddingKey(&req)
		if cacheKey != "" {
			if entry, err := h.cache.GetEmbedding(r.Context(), cacheKey); err != nil {
				h.logger.Warn("embedding cache get failed", "error", err)
			} else if entry != nil {
				telemetry.SetCacheResult(r.Context(), "hit")
				telemetry.SetModel(r.Context(), req.Model, parsed.Provider)
				w.Header().Set("X-Cache", "HIT")
				writeJSON(w, http.StatusOK, entry.Response)
				return
			}
		}
	}

	// Call provider.
	resp, err := ep.Embed(r.Context(), parsed.Model, &req)
	if err != nil {
		h.logger.Error("embedding provider error", "provider", parsed.Provider, "error", err)
		var gwErr *provider.GatewayError
		if errors.As(err, &gwErr) {
			writeError(w, gwErr.HTTPStatus, gwErr.Message, string(gwErr.Code), "")
		} else {
			writeError(w, http.StatusBadGateway, "upstream network error", "api_error", "")
		}
		return
	}

	// Cost calculation.
	var costUSD float64
	if h.costCalc != nil && resp.Usage != nil {
		costUSD = h.costCalc.Calculate(req.Model, resp.Usage.TotalTokens, 0)
	}

	// Cache store (errors are non-fatal).
	if h.cache != nil && cacheKey != "" {
		if storeErr := h.cache.StoreEmbedding(r.Context(), cacheKey, resp, costUSD, 0); storeErr != nil {
			if !errors.Is(storeErr, exactcache.ErrResponseTooLarge) {
				h.logger.Warn("embedding cache store failed", "error", storeErr)
			}
		}
	}

	// Telemetry.
	telemetry.SetModel(r.Context(), req.Model, parsed.Provider)
	if resp.Usage != nil {
		telemetry.SetTokens(r.Context(), resp.Usage.TotalTokens, 0, resp.Usage.TotalTokens)
	}

	writeJSON(w, http.StatusOK, resp)
}
