package mcp

import "encoding/json"

// Protocol version negotiated during initialize.
const ProtocolVersion = "2024-11-05"

// --- JSON-RPC 2.0 envelope types ---

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Standard JSON-RPC error codes.
const (
	ErrParse          = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternal       = -32603

	// MCP-specific errors (app-level).
	ErrPolicyDenied   = -32001
	ErrServerNotFound = -32002
	ErrToolNotFound   = -32003
	ErrTimeout        = -32004
	ErrResultTooLarge = -32005
)

// --- MCP domain types ---

// Tool describes a callable tool provided by an MCP server.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	// Server is the registry name of the MCP server that owns this tool (added by hub).
	Server string `json:"server,omitempty"`
}

// Resource is a readable resource exposed by an MCP server.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	// Server is added by hub.
	Server string `json:"server,omitempty"`
}

// Prompt is a re-usable prompt template from an MCP server.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	// Server is added by hub.
	Server string `json:"server,omitempty"`
}

// PromptArgument describes one parameter of a prompt template.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ToolContent is a single piece of content returned by a tool call.
type ToolContent struct {
	Type string `json:"type"` // "text" | "image" | "resource"
	Text string `json:"text,omitempty"`
	// Additional fields for image/resource content types.
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`
}

// --- Initialize ---

// InitializeParams are sent by the client in the initialize call.
type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

// InitializeResult is returned by the hub.
type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

// --- HTTP request/response types for the Gateway's public MCP API ---

// ToolCallRequest is the body for POST /mcp/v1/tools/call.
type ToolCallRequest struct {
	Server    string         `json:"server"`              // upstream MCP server name
	Tool      string         `json:"tool"`                // tool name
	Arguments map[string]any `json:"arguments,omitempty"` // tool input
}

// ToolCallResponse is returned by POST /mcp/v1/tools/call.
type ToolCallResponse struct {
	Content  []ToolContent `json:"content"`
	IsError  bool          `json:"isError,omitempty"`
	Cached   bool          `json:"cached,omitempty"`
	DurationMS int64       `json:"duration_ms"`
}

// ResourceReadRequest is the body for POST /mcp/v1/resources/read.
type ResourceReadRequest struct {
	Server string `json:"server"`
	URI    string `json:"uri"`
}

// PromptGetRequest is the body for POST /mcp/v1/prompts/get.
type PromptGetRequest struct {
	Server    string            `json:"server"`
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

// ListToolsRequest is the optional filter for POST /mcp/v1/tools/list.
type ListToolsRequest struct {
	// Server filters results to a single upstream server (optional).
	Server string `json:"server,omitempty"`
}

// --- Server configuration types ---

// ServerConfig describes one upstream MCP server.
type ServerConfig struct {
	// Name is the unique registry identifier (e.g. "filesystem", "postgres").
	Name string `koanf:"name"`

	// Type is the transport: "stdio", "sse", or "websocket".
	Type string `koanf:"type"`

	// --- stdio fields ---
	Command string            `koanf:"command"`
	Args    []string          `koanf:"args"`
	Env     map[string]string `koanf:"env"`

	// --- sse / websocket fields ---
	URL    string `koanf:"url"`
	APIKey string `koanf:"api_key"`

	// Auth for websocket (bearer token, etc.)
	Auth ServerAuthConfig `koanf:"auth"`
}

// ServerAuthConfig holds authentication details for remote MCP servers.
type ServerAuthConfig struct {
	Type  string `koanf:"type"`  // "bearer", "basic"
	Token string `koanf:"token"` // bearer token or password
	User  string `koanf:"user"`  // basic auth username
}
