package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// AuditLogger records MCP tool call events.
type AuditLogger interface {
	LogToolCall(ctx context.Context, ev ToolCallEvent)
}

// ToolCallEvent captures data about a single MCP tool execution.
type ToolCallEvent struct {
	Server        string
	Tool          string
	Arguments     map[string]any
	ResultSize    int
	DurationMS    int64
	Error         string
	VirtualKeyID  string
	RequestID     string
	Timestamp     time.Time
}

// Proxy wraps the Hub with policy enforcement, result size limits,
// optional tool-result caching, and audit logging.
type Proxy struct {
	hub    *Hub
	logger *slog.Logger
	audit  AuditLogger // optional

	// cache is an optional in-memory result cache keyed by (server, tool, argsHash).
	cache *resultCache
}

// NewProxy creates a Proxy around hub.
func NewProxy(hub *Hub, logger *slog.Logger, audit AuditLogger, cacheTTL time.Duration) *Proxy {
	p := &Proxy{
		hub:    hub,
		logger: logger,
		audit:  audit,
	}
	if cacheTTL > 0 {
		p.cache = newResultCache(cacheTTL)
	}
	return p
}

// Initialize responds with the Gateway's MCP server info.
func (p *Proxy) Initialize(_ context.Context, _ *InitializeParams) *InitializeResult {
	result := &InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: map[string]any{
			"tools":     map[string]any{},
			"resources": map[string]any{},
			"prompts":   map[string]any{},
		},
	}
	result.ServerInfo.Name = "llm-router-mcp-hub"
	result.ServerInfo.Version = "1.0.0"
	return result
}

// ListTools returns tools from all servers (or a specific one) filtered by policy.
func (p *Proxy) ListTools(ctx context.Context, serverFilter string, policy *Policy) []Tool {
	if serverFilter != "" {
		s, err := p.hub.Server(serverFilter)
		if err != nil {
			return nil
		}
		tools, err := s.ListTools(ctx)
		if err != nil {
			p.logger.Warn("mcp proxy: ListTools error", "server", serverFilter, "error", err)
			return nil
		}
		result := make([]Tool, 0, len(tools))
		for _, t := range tools {
			t.Server = serverFilter
			if policy.CanCallTool(serverFilter, t.Name) {
				result = append(result, t)
			}
		}
		return result
	}

	all := p.hub.ListAllTools(ctx)
	result := make([]Tool, 0, len(all))
	for _, t := range all {
		if policy.CanCallTool(t.Server, t.Name) {
			result = append(result, t)
		}
	}
	return result
}

// CallTool executes a tool after checking the policy and size limits.
func (p *Proxy) CallTool(ctx context.Context, req *ToolCallRequest, policy *Policy, virtualKeyID, requestID string) (*ToolCallResponse, error) {
	if !policy.CanCallTool(req.Server, req.Tool) {
		return nil, &PolicyError{Server: req.Server, Tool: req.Tool}
	}
	if policy.NeedsApproval(req.Tool) {
		return nil, fmt.Errorf("tool %q on %q requires human approval", req.Tool, req.Server)
	}

	// Check cache.
	if p.cache != nil {
		if cached, ok := p.cache.Get(req); ok {
			return &ToolCallResponse{
				Content: cached,
				Cached:  true,
			}, nil
		}
	}

	start := time.Now()
	raw, err := p.hub.CallTool(ctx, req.Server, req.Tool, req.Arguments)
	elapsed := time.Since(start)

	// Audit log regardless of error.
	ev := ToolCallEvent{
		Server:       req.Server,
		Tool:         req.Tool,
		Arguments:    req.Arguments,
		DurationMS:   elapsed.Milliseconds(),
		VirtualKeyID: virtualKeyID,
		RequestID:    requestID,
		Timestamp:    time.Now().UTC(),
	}

	if err != nil {
		ev.Error = err.Error()
		if p.audit != nil {
			p.audit.LogToolCall(ctx, ev)
		}
		return nil, fmt.Errorf("mcp: CallTool %s/%s: %w", req.Server, req.Tool, err)
	}

	ev.ResultSize = len(raw)
	if p.audit != nil {
		p.audit.LogToolCall(ctx, ev)
	}

	// Size limit.
	maxSize := policy.EffectiveMaxResultSize()
	if len(raw) > maxSize {
		return nil, fmt.Errorf("tool result size %d exceeds limit %d", len(raw), maxSize)
	}

	// Parse content.
	var callResult struct {
		Content []ToolContent `json:"content"`
		IsError bool          `json:"isError"`
	}
	if err := json.Unmarshal(raw, &callResult); err != nil {
		// Wrap as plain text if parsing fails.
		callResult.Content = []ToolContent{{Type: "text", Text: string(raw)}}
	}

	// Store in cache (only non-error results).
	if p.cache != nil && !callResult.IsError {
		p.cache.Set(req, callResult.Content)
	}

	return &ToolCallResponse{
		Content:    callResult.Content,
		IsError:    callResult.IsError,
		DurationMS: elapsed.Milliseconds(),
	}, nil
}

// ListResources returns resources from all servers filtered by policy.
func (p *Proxy) ListResources(ctx context.Context, serverFilter string, policy *Policy) []Resource {
	if serverFilter != "" {
		if !policy.CanAccessServer(serverFilter) {
			return nil
		}
		s, err := p.hub.Server(serverFilter)
		if err != nil {
			return nil
		}
		resources, _ := s.ListResources(ctx)
		for i := range resources {
			resources[i].Server = serverFilter
		}
		return resources
	}

	all := p.hub.ListAllResources(ctx)
	result := make([]Resource, 0, len(all))
	for _, r := range all {
		if policy.CanAccessServer(r.Server) {
			result = append(result, r)
		}
	}
	return result
}

// ReadResource reads a resource after checking the policy.
func (p *Proxy) ReadResource(ctx context.Context, serverName, uri string, policy *Policy) (json.RawMessage, error) {
	if !policy.CanAccessServer(serverName) {
		return nil, &PolicyError{Server: serverName}
	}
	return p.hub.ReadResource(ctx, serverName, uri)
}

// ListPrompts returns prompts from all servers filtered by policy.
func (p *Proxy) ListPrompts(ctx context.Context, serverFilter string, policy *Policy) []Prompt {
	if serverFilter != "" {
		if !policy.CanAccessServer(serverFilter) {
			return nil
		}
		s, err := p.hub.Server(serverFilter)
		if err != nil {
			return nil
		}
		prompts, _ := s.ListPrompts(ctx)
		for i := range prompts {
			prompts[i].Server = serverFilter
		}
		return prompts
	}

	all := p.hub.ListAllPrompts(ctx)
	result := make([]Prompt, 0, len(all))
	for _, pr := range all {
		if policy.CanAccessServer(pr.Server) {
			result = append(result, pr)
		}
	}
	return result
}

// GetPrompt fetches a prompt after checking the policy.
func (p *Proxy) GetPrompt(ctx context.Context, serverName, name string, args map[string]string, policy *Policy) (json.RawMessage, error) {
	if !policy.CanAccessServer(serverName) {
		return nil, &PolicyError{Server: serverName}
	}
	return p.hub.GetPrompt(ctx, serverName, name, args)
}

// PolicyError is returned when the caller's policy denies an operation.
type PolicyError struct {
	Server string
	Tool   string
}

func (e *PolicyError) Error() string {
	if e.Tool != "" {
		return fmt.Sprintf("policy denies access to tool %q on server %q", e.Tool, e.Server)
	}
	return fmt.Sprintf("policy denies access to server %q", e.Server)
}

// --- simple TTL cache ---

type cacheEntry struct {
	content   []ToolContent
	expiresAt time.Time
}

type resultCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

func newResultCache(ttl time.Duration) *resultCache {
	c := &resultCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
	return c
}

func (c *resultCache) key(req *ToolCallRequest) string {
	b, _ := json.Marshal(req)
	return string(b)
}

func (c *resultCache) Get(req *ToolCallRequest) ([]ToolContent, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[c.key(req)]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.content, true
}

func (c *resultCache) Set(req *ToolCallRequest, content []ToolContent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[c.key(req)] = cacheEntry{
		content:   content,
		expiresAt: time.Now().Add(c.ttl),
	}
}
