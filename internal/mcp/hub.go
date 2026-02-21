package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
)

// Hub manages a set of upstream MCP servers and aggregates their
// tools, resources, and prompts into a single namespace.
type Hub struct {
	mu      sync.RWMutex
	servers map[string]Server
	logger  *slog.Logger
}

// NewHub creates an empty Hub.
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		servers: make(map[string]Server),
		logger:  logger,
	}
}

// Register adds a server to the hub. If the server is already registered it is replaced.
func (h *Hub) Register(s Server) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.servers[s.Name()] = s
}

// Connect calls Connect on all registered servers and logs errors (non-fatal).
func (h *Hub) Connect(ctx context.Context) {
	h.mu.RLock()
	srvs := make([]Server, 0, len(h.servers))
	for _, s := range h.servers {
		srvs = append(srvs, s)
	}
	h.mu.RUnlock()

	var wg sync.WaitGroup
	for _, s := range srvs {
		wg.Add(1)
		go func(s Server) {
			defer wg.Done()
			if err := s.Connect(ctx); err != nil {
				h.logger.Error("mcp: failed to connect to server", "server", s.Name(), "error", err)
			}
		}(s)
	}
	wg.Wait()
}

// Close terminates all server connections.
func (h *Hub) Close() {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.servers {
		if err := s.Close(); err != nil {
			h.logger.Warn("mcp: error closing server", "server", s.Name(), "error", err)
		}
	}
}

// ServerNames returns the names of all registered servers.
func (h *Hub) ServerNames() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	names := make([]string, 0, len(h.servers))
	for name := range h.servers {
		names = append(names, name)
	}
	return names
}

// Server returns the named server or an error if not found.
func (h *Hub) Server(name string) (Server, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.servers[name]
	if !ok {
		return nil, fmt.Errorf("mcp server %q not found", name)
	}
	return s, nil
}

// ListAllTools returns tools from all servers, tagged with their server name.
func (h *Hub) ListAllTools(ctx context.Context) []Tool {
	h.mu.RLock()
	srvs := snapshotServers(h.servers)
	h.mu.RUnlock()

	var mu sync.Mutex
	var result []Tool
	var wg sync.WaitGroup

	for name, s := range srvs {
		wg.Add(1)
		go func(name string, s Server) {
			defer wg.Done()
			tools, err := s.ListTools(ctx)
			if err != nil {
				h.logger.Warn("mcp: ListTools error", "server", name, "error", err)
				return
			}
			mu.Lock()
			for _, t := range tools {
				t.Server = name
				result = append(result, t)
			}
			mu.Unlock()
		}(name, s)
	}
	wg.Wait()
	return result
}

// CallTool routes a tool call to the named server.
func (h *Hub) CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (json.RawMessage, error) {
	s, err := h.Server(serverName)
	if err != nil {
		return nil, err
	}
	return s.CallTool(ctx, toolName, args)
}

// ListAllResources aggregates resources from all servers.
func (h *Hub) ListAllResources(ctx context.Context) []Resource {
	h.mu.RLock()
	srvs := snapshotServers(h.servers)
	h.mu.RUnlock()

	var mu sync.Mutex
	var result []Resource
	var wg sync.WaitGroup

	for name, s := range srvs {
		wg.Add(1)
		go func(name string, s Server) {
			defer wg.Done()
			resources, err := s.ListResources(ctx)
			if err != nil {
				h.logger.Warn("mcp: ListResources error", "server", name, "error", err)
				return
			}
			mu.Lock()
			for _, r := range resources {
				r.Server = name
				result = append(result, r)
			}
			mu.Unlock()
		}(name, s)
	}
	wg.Wait()
	return result
}

// ReadResource reads a resource from the named server.
func (h *Hub) ReadResource(ctx context.Context, serverName, uri string) (json.RawMessage, error) {
	s, err := h.Server(serverName)
	if err != nil {
		return nil, err
	}
	return s.ReadResource(ctx, uri)
}

// ListAllPrompts aggregates prompts from all servers.
func (h *Hub) ListAllPrompts(ctx context.Context) []Prompt {
	h.mu.RLock()
	srvs := snapshotServers(h.servers)
	h.mu.RUnlock()

	var mu sync.Mutex
	var result []Prompt
	var wg sync.WaitGroup

	for name, s := range srvs {
		wg.Add(1)
		go func(name string, s Server) {
			defer wg.Done()
			prompts, err := s.ListPrompts(ctx)
			if err != nil {
				h.logger.Warn("mcp: ListPrompts error", "server", name, "error", err)
				return
			}
			mu.Lock()
			for _, p := range prompts {
				p.Server = name
				result = append(result, p)
			}
			mu.Unlock()
		}(name, s)
	}
	wg.Wait()
	return result
}

// GetPrompt fetches a prompt from the named server.
func (h *Hub) GetPrompt(ctx context.Context, serverName, name string, args map[string]string) (json.RawMessage, error) {
	s, err := h.Server(serverName)
	if err != nil {
		return nil, err
	}
	return s.GetPrompt(ctx, name, args)
}

// HealthAll returns per-server health status.
func (h *Hub) HealthAll(ctx context.Context) map[string]string {
	h.mu.RLock()
	srvs := snapshotServers(h.servers)
	h.mu.RUnlock()

	result := make(map[string]string, len(srvs))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for name, s := range srvs {
		wg.Add(1)
		go func(name string, s Server) {
			defer wg.Done()
			status := "ok"
			if err := s.Health(ctx); err != nil {
				status = err.Error()
			}
			mu.Lock()
			result[name] = status
			mu.Unlock()
		}(name, s)
	}
	wg.Wait()
	return result
}

func snapshotServers(m map[string]Server) map[string]Server {
	cp := make(map[string]Server, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
