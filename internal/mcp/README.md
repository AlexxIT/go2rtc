# MCP Server Module

The MCP (Model Context Protocol) server module allows go2rtc to be controlled remotely through the MCP protocol. This enables AI assistants like Claude to interact with go2rtc directly.

## Overview

MCP is an open protocol standardised by Anthropic that enables AI assistants to interact with local and remote resources through a structured interface. The go2rtc MCP server exposes stream management, configuration access, and diagnostics as MCP tools, resources, and prompts.

## Configuration

Enable the MCP module in your go2rtc configuration file:

```yaml
mcp:
  enabled: true    # Enable MCP server (default: false)
  http: true       # Enable HTTP transport (default: true)
  sse: false       # Enable Server-Sent Events transport (default: false)
  websocket: false # Enable WebSocket transport (default: false)
  unix: ""         # Optional: Unix socket path
  proxy: ""        # Optional: upstream URL for CLI proxy mode (-mcp)
```

### Transport Options

#### CLI Proxy Mode (`-mcp`)
Runs a lightweight stdio MCP proxy process and forwards all JSON-RPC calls to an
already running go2rtc HTTP MCP endpoint (default: `http://127.0.0.1:1984/mcp`).

You can override upstream with either CLI flag or config:

```yaml
mcp:
  proxy: "http://127.0.0.1:1984/mcp"
```

#### HTTP Transport
Enables MCP over HTTP POST requests. Useful for web-based integrations.

```yaml
mcp:
  enabled: true
  http: true
```

The HTTP endpoint is available at `http://localhost:1984/mcp` (or your configured API port).

Example HTTP request:

```bash
curl -X POST http://localhost:1984/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list"
  }'
```

#### SSE Transport
Server-Sent Events for real-time bidirectional communication.

```yaml
mcp:
  enabled: true
  sse: true
```

The SSE endpoint is available at `http://localhost:1984/mcp/sse`.

SSE flow:
1. `GET /mcp/sse` opens event stream.
2. Server sends `endpoint` event with path like `/mcp/sse?sessionId=...`.
3. Client sends JSON-RPC via `POST` to that endpoint.
4. Server pushes JSON-RPC responses back as SSE `message` events.

#### WebSocket Transport
WebSocket for real-time bidirectional communication with lower overhead.

```yaml
mcp:
  enabled: true
  websocket: true
```

The WebSocket endpoint is available at `ws://localhost:1984/mcp/ws`.

WebSocket flow:
1. Client establishes WebSocket connection to `/mcp/ws`.
2. Both JSON-RPC requests and responses are sent as text messages over WebSocket.
3. Server sends periodic pings to keep connection alive.

#### Unix Socket Transport
Unix socket for local communication with enhanced security.

```yaml
mcp:
  enabled: true
  unix: "/tmp/go2rtc-mcp.sock"
```

The Unix socket transport is useful for local communication without network overhead.

## MCP Tools

Tools are functions that the AI assistant can call to perform actions.

### `list_streams`
List all configured streams in go2rtc.

**Parameters:**
```json
{
  "details": false  // Optional: Set to true for detailed producer/consumer information
}
```

**Returns:**
```json
{
  "count": 3,
  "streams": {
    "camera1": ["rtsp://..."],
    "camera2": ["http://..."]
  }
}
```

### `get_stream`
Get detailed information about a specific stream.

**Parameters:**
```json
{
  "name": "camera"  // Required: Stream name
}
```

**Returns:**
```json
{
  "name": "camera",
  "sources": ["rtsp://..."],
  "producers": [...],
  "details": {...}
}
```

### `add_stream`
Add or update a stream in go2rtc.

**Parameters:**
```json
{
  "name": "camera",           // Required: Stream name
  "url": "rtsp://..."         // Required: Stream source URL
}
```

**Returns:**
```json
{
  "success": true,
  "name": "camera",
  "url": "rtsp://...",
  "sources": ["rtsp://..."]
}
```

### `delete_stream`
Delete a stream from go2rtc.

**Parameters:**
```json
{
  "name": "camera"  // Required: Stream name to delete
}
```

**Returns:**
```json
{
  "success": true,
  "name": "camera"
}
```

### `get_config`
Get current go2rtc configuration.

**Parameters:**
```json
{
  "section": "streams"  // Optional: Specific config section
}
```

**Returns:**
```json
{
  "info": {...},
  "config": {...}  // If section specified
}
```

### `get_info`
Get go2rtc server information (version, host, etc.).

**Returns:**
```json
{
  "version": "1.9.14",
  "host": "localhost:1984",
  ...
}
```

### Additional go2rtc tools

- `list_schemes` - list all supported producer/consumer schemes.
- `validate_source` - validate a URL/expression and report support/validation status.
- `play_stream` - push media from `source` into an existing stream.
- `publish_stream` - publish an existing stream to a destination URL.
- `list_preloads` - list active preload sessions.
- `add_preload` - create/update preload for a stream.
- `delete_preload` - remove preload for a stream.
- `get_streams_dot` - generate Graphviz DOT topology for one/all streams.

### `get_stream_urls`
Get all available output URLs for a stream.

**Parameters:**
```json
{
  "name": "camera",      // Required: Stream name
  "scheme": "hls"        // Optional: Get URL for specific scheme only
}
```

**Returns:**
```json
{
  "name": "camera",
  "host": "http://localhost:1984",
  "urls": {
    "mp4": "http://localhost:1984/mp4/camera.mp4",
    "hls": "http://localhost:1984/hls/camera.m3u8",
    "webrtc": "http://localhost:1984/api/stream?src=camera",
    "mse": "http://localhost:1984/mse/camera",
    "rtsp": "rtsp://localhost:8554/camera",
    ...
  }
}
```

### `get_stream_consumers`
Get information about active consumers connected to a stream.

**Parameters:**
```json
{
  "name": "camera"  // Required: Stream name
}
```

**Returns:**
```json
{
  "name": "camera",
  "count": 2,
  "consumers": [...]
}
```

### `restart_stream`
Restart a stream by stopping all producers and restarting them.

**Parameters:**
```json
{
  "name": "camera"  // Required: Stream name to restart
}
```

**Returns:**
```json
{
  "success": true,
  "name": "camera",
  "sources": ["rtsp://..."]
}
```

### `get_events`
Get recent events from the event log.

**Parameters:**
```json
{
  "count": 10  // Optional: Number of events (default: 10)
}
```

**Returns:**
```json
{
  "count": 10,
  "events": [
    {
      "type": "stream_added",
      "timestamp": "2024-01-01T12:00:00Z",
      "data": {"name": "camera", "sources": [...]}
    }
  ]
}
```

### `get_connections`
Get summary of all stream connections.

**Returns:**
```json
{
  "connections": [
    {"name": "camera", "producers": 1, "consumers": 2},
    ...
  ]
}
```

### `get_stream_stats`
Get detailed statistics for a specific stream.

**Parameters:**
```json
{
  "name": "camera"  // Required: Stream name
}
```

**Returns:**
```json
{
  "name": "camera",
  "sources": ["rtsp://..."],
  "producers": [{"mode": "active", "state": "online"}],
  "consumer_count": 2
}
```

### `stream_snapshot`
Get a complete snapshot of a stream's current state.

**Parameters:**
```json
{
  "name": "camera"  // Required: Stream name
}
```

**Returns:**
```json
{
  "name": "camera",
  "sources": ["rtsp://..."],
  "timestamp": "2024-01-01T12:00:00Z",
  "details": {...}
}
```

## MCP Resources

Resources are read-only data sources that the AI assistant can access.

### `streams://`
List of all configured streams with producer details.

**Content-Type:** `application/json`

**Example:**
```json
{
  "count": 2,
  "camera1": {
    "sources": ["rtsp://..."],
    "producers": [...]
  }
}
```

### `streams://{name}`
Detailed information about a specific stream.

**Content-Type:** `application/json`

**Example URI:** `streams://camera`

### `log://`
Current server log.

**Content-Type:** `application/jsonlines`

Returns the in-memory log entries from go2rtc.

### `events://`
Recent events from the event log.

**Content-Type:** `application/json`

Returns a log of recent events including stream additions, updates, and removals.

## MCP Prompts

Prompts are pre-defined templates that guide the AI assistant in performing specific tasks.

### `stream_analyzer`
Analyzes a go2rtc stream for issues and provides diagnostic information.

**Parameters:**
```json
{
  "name": "camera"  // Required: Stream name to analyze
}
```

**Returns:**
A formatted analysis including:
- Stream sources
- Producer states
- Analysis points for troubleshooting

### Additional prompts

- `stream_setup_wizard` - generate step-by-step setup plan for a new stream (`name`, `source`).
- `operations_playbook` - generate maintenance/troubleshooting playbook (optional `name`).
- `source_configurator` - get configuration guide for specific source types (RTSP, HTTP, RTMP, FFmpeg, WebRTC, etc.).
- `troubleshooting_guide` - get step-by-step troubleshooting instructions for common issues.
- `integration_helper` - get integration guides for Home Assistant, Frigate, HomeBridge, MQTT, NVRs, etc.
- `performance_optimizer` - get optimization recommendations for low latency, bandwidth, CPU usage, or reliability.
- `security_auditor` - generate security checklist and audit go2rtc configuration.
- `backup_restore` - get instructions for backing up and restoring go2rtc configuration.
- `migration_helper` - get migration guides from direct RTSP/FFmpeg/MQTT to go2rtc.

## Usage Examples

### 1) Claude Desktop (local stdio proxy)

Best when Claude Desktop and go2rtc run on the same machine.

1. Enable HTTP MCP transport in your main go2rtc service:

```yaml
mcp:
  enabled: true
  http: true
```

2. Configure Claude Desktop (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS)
to start a lightweight stdio proxy process:

```json
{
  "mcpServers": {
    "go2rtc": {
      "command": "/path/to/go2rtc",
      "args": ["-mcp", "-c", "/path/to/go2rtc.yaml"]
    }
  }
}
```

By default, `-mcp` proxies to `http://127.0.0.1:1984/mcp`.
You can override upstream endpoint with `-mcp-url`, for example:

```json
{
  "mcpServers": {
    "go2rtc": {
      "command": "/path/to/go2rtc",
      "args": ["-mcp", "-mcp-url", "http://127.0.0.1:1990/mcp"]
    }
  }
}
```

3. Restart Claude Desktop.

4. Example prompts:
   - "List all streams"
   - "Add stream `front_door` from `rtsp://192.168.1.100:554/stream1`"
   - "Analyze stream `front_door`"

### 2) Direct HTTP (curl / scripts)

Enable HTTP transport:

```yaml
mcp:
  enabled: true
  http: true
```

Then call MCP at `http://localhost:1984/mcp`:

```bash
# Initialize session
curl -X POST http://localhost:1984/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","clientInfo":{"name":"curl","version":"1.0.0"}}}'

# Notify initialized
curl -X POST http://localhost:1984/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}'

# List tools
curl -X POST http://localhost:1984/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'

# Call a tool
curl -X POST http://localhost:1984/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_streams","arguments":{"details":true}}}'

# List resources
curl -X POST http://localhost:1984/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":4,"method":"resources/list"}'

# Read a resource
curl -X POST http://localhost:1984/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"streams://"}}'
```

### 3) HTTP in popular AI apps (remote go2rtc)

Many desktop AI apps configure MCP servers as local commands (stdio).  
To connect those apps to the HTTP endpoint, use an MCP HTTP bridge:

```bash
npx -y mcp-remote http://localhost:1984/mcp
```

Use this command in the app's MCP server config.

#### Claude Desktop (HTTP via bridge)

```json
{
  "mcpServers": {
    "go2rtc-http": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "http://localhost:1984/mcp"]
    }
  }
}
```

#### Cursor (HTTP via bridge)

`~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "go2rtc-http": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "http://localhost:1984/mcp"]
    }
  }
}
```

#### Cline in VS Code (HTTP via bridge)

Add an MCP server in Cline settings with:
- Command: `npx`
- Args: `-y mcp-remote http://localhost:1984/mcp`

### 4) Batch request over HTTP

Send multiple MCP calls in a single HTTP request:

```bash
curl -X POST http://localhost:1984/mcp \
  -H "Content-Type: application/json" \
  -d '[
    {"jsonrpc":"2.0","id":10,"method":"tools/list"},
    {"jsonrpc":"2.0","id":11,"method":"resources/list"}
  ]'
```

## Protocol Details

The go2rtc MCP server implements the MCP protocol version `2024-11-05` with the following capabilities:

- **Tools**: Execute actions on go2rtc
- **Resources**: Read configuration and state
- **Prompts**: Pre-defined analysis templates
- **Standard methods**: supports `ping` and initialized/cancel notifications

### Error Codes

The server uses standard JSON-RPC error codes:

| Code | Name | Description |
|------|------|-------------|
| -32700 | Parse error | Invalid JSON |
| -32600 | Invalid request | Invalid JSON-RPC request |
| -32601 | Method not found | Unknown MCP method |
| -32602 | Invalid params | Invalid method parameters |
| -32603 | Internal error | Server-side error |

## Security Considerations

When enabling the MCP module:

1. **HTTP transport** inherits authentication from go2rtc's API configuration
2. **CLI proxy mode (`-mcp`)** is only accessible to processes that can execute go2rtc
3. **SSE transport** requires proper CORS configuration for web access

Always secure your go2rtc instance with authentication when exposing MCP over HTTP:

```yaml
api:
  username: admin
  password: your-secure-password
```

## Development

### Running Tests

```bash
go test ./internal/mcp/... -v
```

### Adding New Tools

To add a new tool, register it in `mcp.go`:

```go
server.RegisterTool("my_tool", Tool{
    Name:        "my_tool",
    Description: "Description of what the tool does",
    InputSchema: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "param": map[string]any{
                "type": "string",
                "description": "Parameter description",
            },
        },
        "required": []string{"param"},
    },
    Handler: handleMyTool,
})
```

Then implement the handler in `handlers.go`:

```go
func handleMyTool(ctx context.Context, args map[string]any) (any, error) {
    // Your implementation
    return map[string]any{"result": "ok"}, nil
}
```

## Troubleshooting

### MCP not appearing in Claude Desktop

1. Check that go2rtc is running with MCP enabled
2. Verify the config file path in Claude Desktop settings
3. Check Claude Desktop logs for connection errors

### HTTP requests returning 405 Method Not Allowed

Ensure you're sending POST requests to `/mcp` endpoint.

### Tools returning errors

Check go2rtc logs for detailed error messages:
```bash
# If running with debug logging
tail -f /path/to/go2rtc.log
```
