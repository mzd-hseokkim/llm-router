package mcp

import "slices"

// Policy defines per-key access control rules for MCP resources.
type Policy struct {
	// AllowedServers restricts which upstream MCP servers are accessible.
	// Empty means all servers are allowed.
	AllowedServers []string `json:"allowed_servers,omitempty"`

	// AllowedTools restricts which tools may be called.
	// Empty means all tools (not in BlockedTools) are allowed.
	AllowedTools []string `json:"allowed_tools,omitempty"`

	// BlockedTools prevents specific tools from being called.
	BlockedTools []string `json:"blocked_tools,omitempty"`

	// RequireApproval lists tools that must be pre-approved before execution.
	// (Reserved for future human-in-the-loop workflows.)
	RequireApproval []string `json:"require_approval,omitempty"`

	// MaxResultSize is the maximum allowed byte size for a tool result.
	// 0 means the default (1 MB).
	MaxResultSize int `json:"max_result_size,omitempty"`
}

const defaultMaxResultSize = 1 << 20 // 1 MB

// EffectiveMaxResultSize returns the configured limit or the default.
func (p *Policy) EffectiveMaxResultSize() int {
	if p == nil || p.MaxResultSize <= 0 {
		return defaultMaxResultSize
	}
	return p.MaxResultSize
}

// CanAccessServer returns true if the policy allows connecting to server.
func (p *Policy) CanAccessServer(server string) bool {
	if p == nil || len(p.AllowedServers) == 0 {
		return true
	}
	return slices.Contains(p.AllowedServers, server)
}

// CanCallTool returns true if the policy permits calling tool on server.
func (p *Policy) CanCallTool(server, tool string) bool {
	if !p.CanAccessServer(server) {
		return false
	}
	if p == nil {
		return true
	}
	if slices.Contains(p.BlockedTools, tool) {
		return false
	}
	if len(p.AllowedTools) > 0 && !slices.Contains(p.AllowedTools, tool) {
		return false
	}
	return true
}

// NeedsApproval returns true if the tool requires human approval.
func (p *Policy) NeedsApproval(tool string) bool {
	if p == nil {
		return false
	}
	return slices.Contains(p.RequireApproval, tool)
}

// DefaultPolicy is an open policy (all servers/tools allowed, default size limit).
var DefaultPolicy = &Policy{}
