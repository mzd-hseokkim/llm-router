package mcp

import (
	"context"
	"log/slog"

	"github.com/llm-router/gateway/internal/audit"
)

// auditLoggerAdapter adapts audit.Logger to the AuditLogger interface.
type auditLoggerAdapter struct {
	logger *audit.Logger
	slog   *slog.Logger
}

// NewAuditAdapter wraps an audit.Logger so that the Proxy can record MCP events.
// If logger is nil, tool calls are only logged via slog.
func NewAuditAdapter(logger *audit.Logger, slog *slog.Logger) AuditLogger {
	return &auditLoggerAdapter{logger: logger, slog: slog}
}

func (a *auditLoggerAdapter) LogToolCall(_ context.Context, ev ToolCallEvent) {
	a.slog.Info("mcp: tool called",
		"server", ev.Server,
		"tool", ev.Tool,
		"duration_ms", ev.DurationMS,
		"result_size", ev.ResultSize,
		"virtual_key_id", ev.VirtualKeyID,
		"request_id", ev.RequestID,
		"error", ev.Error,
	)

	if a.logger == nil {
		return
	}

	eventType := audit.EventMCPToolCalled
	if ev.Error != "" {
		eventType = audit.EventMCPToolFailed
	}

	meta := map[string]any{
		"server":          ev.Server,
		"tool":            ev.Tool,
		"result_size":     ev.ResultSize,
		"duration_ms":     ev.DurationMS,
		"virtual_key_id":  ev.VirtualKeyID,
	}
	if ev.Error != "" {
		meta["error"] = ev.Error
	}

	a.logger.Record(&audit.Event{
		EventType:    eventType,
		Action:       "tool_call",
		ActorType:    audit.ActorAPIKey,
		ActorID:      nil,
		ResourceType: "mcp_tool",
		ResourceName: ev.Server + "/" + ev.Tool,
		Metadata:     meta,
		RequestID:    ev.RequestID,
		Timestamp:    ev.Timestamp,
	})
}
