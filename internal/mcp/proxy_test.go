package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"
)

// mockServer is a test double for the Server interface.
type mockServer struct {
	name      string
	tools     []Tool
	resources []Resource
	prompts   []Prompt
	callErr   error
}

func (m *mockServer) Name() string                { return m.name }
func (m *mockServer) Connect(_ context.Context) error { return nil }
func (m *mockServer) Close() error                    { return nil }
func (m *mockServer) Health(_ context.Context) error  { return nil }

func (m *mockServer) ListTools(_ context.Context) ([]Tool, error) {
	return m.tools, nil
}

func (m *mockServer) CallTool(_ context.Context, name string, args map[string]any) (json.RawMessage, error) {
	if m.callErr != nil {
		return nil, m.callErr
	}
	result := map[string]any{
		"content": []map[string]any{{"type": "text", "text": "ok: " + name}},
		"isError": false,
	}
	b, _ := json.Marshal(result)
	return b, nil
}

func (m *mockServer) ListResources(_ context.Context) ([]Resource, error) {
	return m.resources, nil
}

func (m *mockServer) ReadResource(_ context.Context, _ string) (json.RawMessage, error) {
	b, _ := json.Marshal(map[string]any{"contents": []any{}})
	return b, nil
}

func (m *mockServer) ListPrompts(_ context.Context) ([]Prompt, error) {
	return m.prompts, nil
}

func (m *mockServer) GetPrompt(_ context.Context, _ string, _ map[string]string) (json.RawMessage, error) {
	b, _ := json.Marshal(map[string]any{"messages": []any{}})
	return b, nil
}

func newTestProxy(t *testing.T, servers ...*mockServer) *Proxy {
	t.Helper()
	hub := NewHub(slog.Default())
	for _, s := range servers {
		hub.Register(s)
	}
	return NewProxy(hub, slog.Default(), nil, 0)
}

// --- Policy tests ---

func TestPolicy_CanCallTool_NoRestrictions(t *testing.T) {
	p := &Policy{}
	if !p.CanCallTool("server1", "any_tool") {
		t.Error("open policy should allow all tools")
	}
}

func TestPolicy_CanCallTool_Blocked(t *testing.T) {
	p := &Policy{BlockedTools: []string{"dangerous_tool"}}
	if p.CanCallTool("server1", "dangerous_tool") {
		t.Error("blocked tool should be denied")
	}
	if !p.CanCallTool("server1", "safe_tool") {
		t.Error("non-blocked tool should be allowed")
	}
}

func TestPolicy_CanCallTool_AllowList(t *testing.T) {
	p := &Policy{AllowedTools: []string{"safe_tool"}}
	if !p.CanCallTool("server1", "safe_tool") {
		t.Error("allowed tool should be permitted")
	}
	if p.CanCallTool("server1", "other_tool") {
		t.Error("tool not in allow list should be denied")
	}
}

func TestPolicy_CanAccessServer_AllowList(t *testing.T) {
	p := &Policy{AllowedServers: []string{"db"}}
	if !p.CanAccessServer("db") {
		t.Error("allowed server should be accessible")
	}
	if p.CanAccessServer("filesystem") {
		t.Error("unlisted server should be denied")
	}
}

func TestPolicy_MaxResultSize_Default(t *testing.T) {
	p := &Policy{}
	if got := p.EffectiveMaxResultSize(); got != defaultMaxResultSize {
		t.Errorf("expected %d, got %d", defaultMaxResultSize, got)
	}
}

// --- Proxy tests ---

func TestProxy_Initialize(t *testing.T) {
	proxy := newTestProxy(t)
	result := proxy.Initialize(context.Background(), &InitializeParams{})
	if result.ProtocolVersion != ProtocolVersion {
		t.Errorf("expected protocol version %s, got %s", ProtocolVersion, result.ProtocolVersion)
	}
	if result.ServerInfo.Name == "" {
		t.Error("server info name should not be empty")
	}
}

func TestProxy_ListTools_AllServers(t *testing.T) {
	s1 := &mockServer{name: "s1", tools: []Tool{{Name: "tool_a"}, {Name: "tool_b"}}}
	s2 := &mockServer{name: "s2", tools: []Tool{{Name: "tool_c"}}}
	proxy := newTestProxy(t, s1, s2)

	tools := proxy.ListTools(context.Background(), "", DefaultPolicy)
	if len(tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(tools))
	}
}

func TestProxy_ListTools_FilterByServer(t *testing.T) {
	s1 := &mockServer{name: "s1", tools: []Tool{{Name: "tool_a"}}}
	s2 := &mockServer{name: "s2", tools: []Tool{{Name: "tool_b"}}}
	proxy := newTestProxy(t, s1, s2)

	tools := proxy.ListTools(context.Background(), "s1", DefaultPolicy)
	if len(tools) != 1 || tools[0].Name != "tool_a" {
		t.Errorf("expected [tool_a] from s1, got %v", tools)
	}
}

func TestProxy_ListTools_PolicyBlocked(t *testing.T) {
	s := &mockServer{name: "s1", tools: []Tool{{Name: "allowed"}, {Name: "blocked"}}}
	proxy := newTestProxy(t, s)

	policy := &Policy{BlockedTools: []string{"blocked"}}
	tools := proxy.ListTools(context.Background(), "", policy)
	for _, t2 := range tools {
		if t2.Name == "blocked" {
			t.Error("blocked tool should not appear in listing")
		}
	}
}

func TestProxy_CallTool_Success(t *testing.T) {
	s := &mockServer{name: "s1", tools: []Tool{{Name: "greet"}}}
	proxy := newTestProxy(t, s)

	req := &ToolCallRequest{Server: "s1", Tool: "greet", Arguments: map[string]any{"name": "world"}}
	resp, err := proxy.CallTool(context.Background(), req, DefaultPolicy, "vk1", "req1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsError {
		t.Error("expected non-error result")
	}
	if len(resp.Content) == 0 {
		t.Error("expected at least one content item")
	}
}

func TestProxy_CallTool_PolicyDenied(t *testing.T) {
	s := &mockServer{name: "s1"}
	proxy := newTestProxy(t, s)

	policy := &Policy{BlockedTools: []string{"dangerous"}}
	req := &ToolCallRequest{Server: "s1", Tool: "dangerous"}
	_, err := proxy.CallTool(context.Background(), req, policy, "", "")
	if err == nil {
		t.Fatal("expected policy error")
	}
	var pe *PolicyError
	if !errors.As(err, &pe) {
		t.Errorf("expected PolicyError, got %T: %v", err, err)
	}
}

func TestProxy_CallTool_ServerNotFound(t *testing.T) {
	proxy := newTestProxy(t)
	req := &ToolCallRequest{Server: "nonexistent", Tool: "tool"}
	_, err := proxy.CallTool(context.Background(), req, DefaultPolicy, "", "")
	if err == nil {
		t.Fatal("expected error for unknown server")
	}
}

func TestProxy_CallTool_Cache(t *testing.T) {
	s := &mockServer{name: "s1", tools: []Tool{{Name: "read"}}}
	hub := NewHub(slog.Default())
	hub.Register(s)
	proxy := NewProxy(hub, slog.Default(), nil, 5*time.Minute)

	req := &ToolCallRequest{Server: "s1", Tool: "read"}
	first, err := proxy.CallTool(context.Background(), req, DefaultPolicy, "", "")
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	if first.Cached {
		t.Error("first call should not be cached")
	}

	second, err := proxy.CallTool(context.Background(), req, DefaultPolicy, "", "")
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if !second.Cached {
		t.Error("second identical call should be served from cache")
	}
}

func TestProxy_CallTool_SizeLimit(t *testing.T) {
	// Returns a result that is larger than the configured limit.
	s := &mockServer{name: "s1"}
	s.tools = []Tool{{Name: "big_result"}}
	// Override CallTool to return a large payload.
	hub := NewHub(slog.Default())
	hub.Register(&bigResultServer{name: "s1"})
	proxy := NewProxy(hub, slog.Default(), nil, 0)

	policy := &Policy{MaxResultSize: 10} // tiny limit
	req := &ToolCallRequest{Server: "s1", Tool: "big"}
	_, err := proxy.CallTool(context.Background(), req, policy, "", "")
	if err == nil {
		t.Fatal("expected size limit error")
	}
}

// bigResultServer always returns a payload larger than a small policy limit.
type bigResultServer struct{ name string }

func (b *bigResultServer) Name() string               { return b.name }
func (b *bigResultServer) Connect(_ context.Context) error { return nil }
func (b *bigResultServer) Close() error                    { return nil }
func (b *bigResultServer) Health(_ context.Context) error  { return nil }
func (b *bigResultServer) ListTools(_ context.Context) ([]Tool, error) {
	return []Tool{{Name: "big"}}, nil
}
func (b *bigResultServer) CallTool(_ context.Context, _ string, _ map[string]any) (json.RawMessage, error) {
	big := make([]byte, 1000)
	payload := map[string]any{"content": []map[string]any{{"type": "text", "text": string(big)}}}
	return json.Marshal(payload)
}
func (b *bigResultServer) ListResources(_ context.Context) ([]Resource, error)              { return nil, nil }
func (b *bigResultServer) ReadResource(_ context.Context, _ string) (json.RawMessage, error) { return nil, nil }
func (b *bigResultServer) ListPrompts(_ context.Context) ([]Prompt, error)                  { return nil, nil }
func (b *bigResultServer) GetPrompt(_ context.Context, _ string, _ map[string]string) (json.RawMessage, error) {
	return nil, nil
}
