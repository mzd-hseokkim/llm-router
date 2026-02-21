package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/llm-router/gateway/internal/mcp"
)

// MCPHandler handles the /mcp/v1/* public endpoints.
// It wraps a Proxy and maps REST calls to MCP operations.
type MCPHandler struct {
	proxy  *mcp.Proxy
	logger *slog.Logger
}

// NewMCPHandler creates an MCPHandler.
func NewMCPHandler(proxy *mcp.Proxy, logger *slog.Logger) *MCPHandler {
	return &MCPHandler{proxy: proxy, logger: logger}
}

// Initialize handles POST /mcp/v1/initialize.
func (h *MCPHandler) Initialize(w http.ResponseWriter, r *http.Request) {
	var params mcp.InitializeParams
	_ = json.NewDecoder(r.Body).Decode(&params) // optional body

	result := h.proxy.Initialize(r.Context(), &params)
	writeJSON(w, http.StatusOK, result)
}

// ListTools handles POST /mcp/v1/tools/list.
func (h *MCPHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	var req mcp.ListToolsRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	policy := policyFromCtx(r.Context())
	tools := h.proxy.ListTools(r.Context(), req.Server, policy)

	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

// CallTool handles POST /mcp/v1/tools/call.
func (h *MCPHandler) CallTool(w http.ResponseWriter, r *http.Request) {
	var req mcp.ToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Server == "" || req.Tool == "" {
		http.Error(w, "server and tool are required", http.StatusBadRequest)
		return
	}

	policy := policyFromCtx(r.Context())
	virtualKeyID := virtualKeyIDFromCtx(r.Context())
	requestID := requestIDFromCtx(r.Context())

	result, err := h.proxy.CallTool(r.Context(), &req, policy, virtualKeyID, requestID)
	if err != nil {
		var pe *mcp.PolicyError
		if errors.As(err, &pe) {
			writeJSONError(w, http.StatusForbidden, mcp.ErrPolicyDenied, err.Error())
			return
		}
		h.logger.Error("mcp: CallTool failed", "server", req.Server, "tool", req.Tool, "error", err)
		writeJSONError(w, http.StatusBadGateway, mcp.ErrInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ListResources handles POST /mcp/v1/resources/list.
func (h *MCPHandler) ListResources(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Server string `json:"server,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	policy := policyFromCtx(r.Context())
	resources := h.proxy.ListResources(r.Context(), req.Server, policy)
	writeJSON(w, http.StatusOK, map[string]any{"resources": resources})
}

// ReadResource handles POST /mcp/v1/resources/read.
func (h *MCPHandler) ReadResource(w http.ResponseWriter, r *http.Request) {
	var req mcp.ResourceReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Server == "" || req.URI == "" {
		http.Error(w, "server and uri are required", http.StatusBadRequest)
		return
	}

	policy := policyFromCtx(r.Context())
	raw, err := h.proxy.ReadResource(r.Context(), req.Server, req.URI, policy)
	if err != nil {
		var pe *mcp.PolicyError
		if errors.As(err, &pe) {
			writeJSONError(w, http.StatusForbidden, mcp.ErrPolicyDenied, err.Error())
			return
		}
		writeJSONError(w, http.StatusBadGateway, mcp.ErrInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, json.RawMessage(raw))
}

// ListPrompts handles POST /mcp/v1/prompts/list.
func (h *MCPHandler) ListPrompts(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Server string `json:"server,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	policy := policyFromCtx(r.Context())
	prompts := h.proxy.ListPrompts(r.Context(), req.Server, policy)
	writeJSON(w, http.StatusOK, map[string]any{"prompts": prompts})
}

// GetPrompt handles POST /mcp/v1/prompts/get.
func (h *MCPHandler) GetPrompt(w http.ResponseWriter, r *http.Request) {
	var req mcp.PromptGetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Server == "" || req.Name == "" {
		http.Error(w, "server and name are required", http.StatusBadRequest)
		return
	}

	policy := policyFromCtx(r.Context())
	raw, err := h.proxy.GetPrompt(r.Context(), req.Server, req.Name, req.Arguments, policy)
	if err != nil {
		var pe *mcp.PolicyError
		if errors.As(err, &pe) {
			writeJSONError(w, http.StatusForbidden, mcp.ErrPolicyDenied, err.Error())
			return
		}
		writeJSONError(w, http.StatusBadGateway, mcp.ErrInternal, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, json.RawMessage(raw))
}

// --- context helpers ---

type mcpPolicyKey struct{}
type mcpVirtualKeyIDKey struct{}
type mcpRequestIDKey struct{}

// WithMCPPolicy stores the resolved MCP policy in the context.
func WithMCPPolicy(ctx context.Context, p *mcp.Policy) context.Context {
	return context.WithValue(ctx, mcpPolicyKey{}, p)
}

func policyFromCtx(ctx context.Context) *mcp.Policy {
	if p, ok := ctx.Value(mcpPolicyKey{}).(*mcp.Policy); ok && p != nil {
		return p
	}
	return mcp.DefaultPolicy
}

func virtualKeyIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(mcpVirtualKeyIDKey{}).(string); ok {
		return v
	}
	return ""
}

func requestIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(mcpRequestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// --- JSON helpers ---

func writeJSONError(w http.ResponseWriter, status, code int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{"code": code, "message": msg},
	})
}
