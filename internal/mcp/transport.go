package mcp

import (
	"context"
	"encoding/json"
)

// Server is the interface implemented by all MCP server transports.
type Server interface {
	// Name returns the registry name of this server.
	Name() string

	// Connect establishes the transport connection and runs the MCP
	// initialize handshake. Must be called before any other method.
	Connect(ctx context.Context) error

	// Close terminates the transport connection.
	Close() error

	// Health returns nil if the server is reachable, an error otherwise.
	Health(ctx context.Context) error

	// ListTools returns all tools advertised by this server.
	ListTools(ctx context.Context) ([]Tool, error)

	// CallTool executes a named tool with the given arguments.
	CallTool(ctx context.Context, name string, args map[string]any) (json.RawMessage, error)

	// ListResources returns all resources exposed by this server.
	ListResources(ctx context.Context) ([]Resource, error)

	// ReadResource reads the resource at the given URI.
	ReadResource(ctx context.Context, uri string) (json.RawMessage, error)

	// ListPrompts returns all prompt templates from this server.
	ListPrompts(ctx context.Context) ([]Prompt, error)

	// GetPrompt renders a prompt template with the given arguments.
	GetPrompt(ctx context.Context, name string, args map[string]string) (json.RawMessage, error)
}

// NewServer creates the appropriate Server implementation based on cfg.Type.
func NewServer(cfg ServerConfig) Server {
	switch cfg.Type {
	case "stdio":
		return newStdioServer(cfg)
	case "sse":
		return newSSEServer(cfg)
	case "websocket":
		return newWSServer(cfg)
	default:
		return newSSEServer(cfg)
	}
}
