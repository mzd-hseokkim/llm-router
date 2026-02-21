# MCP Gateway Setup Guide

LLM Router acts as a central **MCP Hub** — a single gateway that aggregates multiple upstream MCP servers and exposes them to LLM clients (Claude Desktop, AI agents, etc.) through a unified HTTP API.

## Architecture

```
Claude Desktop / LLM Agent
         ↓  (HTTP POST — virtual key auth)
  LLM Router MCP Hub  (/mcp/v1/*)
         ↓  (transport-specific)
   ┌─────┼─────┐
  stdio  SSE  WebSocket
   │     │      │
  npx  remote  ws://
  cmd  server  server
```

## Quick Start

### 1. Enable MCP in config.yaml

```yaml
mcp:
  enabled: true
  tool_cache_ttl: 30s   # cache idempotent results (0 = disabled)
  servers:
    - name: filesystem
      type: stdio
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/data"]
```

See `config/mcp-servers.yaml` for a complete example with all transport types.

### 2. Get a Virtual Key

```bash
curl -X POST http://localhost:8080/admin/keys \
  -H "Authorization: Bearer $MASTER_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name": "claude-desktop", "rpm_limit": 60}'
```

### 3. Configure Claude Desktop

Add to `~/.config/claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "llm-router": {
      "url": "http://localhost:8080/mcp/v1",
      "apiKey": "sk-router-..."
    }
  }
}
```

---

## API Reference

All endpoints require a Virtual Key in the `Authorization: Bearer <key>` header.

### POST /mcp/v1/initialize

Initialize an MCP session and discover server capabilities.

**Request body** (optional):
```json
{
  "protocolVersion": "2024-11-05",
  "clientInfo": { "name": "my-agent", "version": "1.0" }
}
```

**Response**:
```json
{
  "protocolVersion": "2024-11-05",
  "capabilities": { "tools": {}, "resources": {}, "prompts": {} },
  "serverInfo": { "name": "llm-router-mcp-hub", "version": "1.0.0" }
}
```

### POST /mcp/v1/tools/list

List all available tools across all connected MCP servers.

**Request body** (optional):
```json
{ "server": "filesystem" }
```
Omit `server` to list tools from all servers.

**Response**:
```json
{
  "tools": [
    {
      "name": "read_file",
      "description": "Read the contents of a file",
      "server": "filesystem",
      "inputSchema": { "type": "object", "properties": { "path": { "type": "string" } } }
    }
  ]
}
```

### POST /mcp/v1/tools/call

Execute a tool on a specific MCP server.

**Request body**:
```json
{
  "server": "filesystem",
  "tool": "read_file",
  "arguments": { "path": "/data/report.txt" }
}
```

**Response**:
```json
{
  "content": [{ "type": "text", "text": "file contents here..." }],
  "isError": false,
  "cached": false,
  "duration_ms": 12
}
```

### POST /mcp/v1/resources/list

List all resources exposed by MCP servers.

### POST /mcp/v1/resources/read

Read a resource by URI.

**Request body**:
```json
{ "server": "postgres", "uri": "postgres://mydb/users/schema" }
```

### POST /mcp/v1/prompts/list

List all prompt templates.

### POST /mcp/v1/prompts/get

Render a prompt template with arguments.

**Request body**:
```json
{
  "server": "filesystem",
  "name": "summarize_file",
  "arguments": { "path": "/data/report.txt" }
}
```

---

## Admin API

All admin endpoints require the master key: `Authorization: Bearer $MASTER_KEY`.

### GET /admin/mcp/servers

List all registered MCP servers with health status.

### POST /admin/mcp/servers

Register a new MCP server at runtime.

```json
{
  "name": "new-server",
  "type": "sse",
  "url": "https://mcp.example.com",
  "api_key": "secret"
}
```

### GET /admin/mcp/servers/{name}/health

Check health of a specific MCP server.

### GET /admin/mcp/servers/{name}/tools

List tools provided by a specific MCP server.

### DELETE /admin/mcp/servers/{name}

Remove and disconnect an MCP server.

### POST /admin/mcp/policies

View the policy schema (policies are applied per virtual key metadata).

---

## Supported Transports

| Type | Description | Use Case |
|------|-------------|----------|
| `stdio` | Child process via stdin/stdout | Local tools (npx, Python scripts) |
| `sse` | HTTP GET (SSE stream) + POST | Remote HTTP MCP servers |
| `websocket` | WebSocket (`ws://`) | Real-time bidirectional servers |

> **Note**: `wss://` (TLS WebSocket) requires additional TLS configuration.

---

## Access Control

Tool access is controlled per virtual key via policy metadata.

Example policy (applied when creating/updating a virtual key):

```json
{
  "mcp_policy": {
    "allowed_servers": ["filesystem", "postgres"],
    "blocked_tools": ["delete_file", "drop_table"],
    "require_approval": ["execute_query"],
    "max_result_size": 524288
  }
}
```

| Field | Description |
|-------|-------------|
| `allowed_servers` | Restrict to specific servers (empty = all allowed) |
| `allowed_tools` | Restrict to specific tools (empty = all allowed) |
| `blocked_tools` | Deny specific tools |
| `require_approval` | Tools that need human approval (future) |
| `max_result_size` | Max response bytes (default: 1 MB) |

---

## Security

- **Process isolation**: Each stdio server runs as a separate child process; failures don't affect other servers.
- **Result size limit**: Tool responses exceeding the limit are rejected (default 1 MB).
- **Execution timeout**: Tool calls time out after 30 seconds.
- **Audit logging**: Every tool call is recorded in the audit log (`mcp.tool_called` / `mcp.tool_failed`).
- **Tool result caching**: Idempotent results are cached (configurable TTL) to reduce upstream load.

---

## Troubleshooting

**Server shows "not connected" in health check:**
- Check the server process is running (`ps aux | grep npx`)
- Verify the URL/command is reachable from the Gateway container
- Check Gateway logs for connection errors

**Tool call returns 403 Forbidden:**
- The virtual key's policy blocks this tool or server
- Review `mcp_policy` in the virtual key settings

**Tool call returns 502 Bad Gateway:**
- The upstream MCP server returned an error or is unreachable
- Check audit logs for `mcp.tool_failed` events
