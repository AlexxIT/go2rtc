package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod struct {
			Enabled  bool   `yaml:"enabled"`
			HTTP     *bool  `yaml:"http"`
			SSE      bool   `yaml:"sse"`
			WebSocket bool  `yaml:"websocket"`
			Unix     string `yaml:"unix"`
		} `yaml:"mcp"`
	}

	app.LoadConfig(&cfg)

	// MCP disabled by default
	if !cfg.Mod.Enabled {
		return
	}

	log = app.GetLogger("mcp")

	// Create main MCP server instance
	server = NewServer()

	// Register MCP tools
	server.RegisterTool("list_streams", Tool{
		Name:        "list_streams",
		Description: "List all configured streams in go2rtc",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"details": map[string]any{
					"type":        "boolean",
					"description": "Include detailed information about each stream",
				},
			},
		},
		Handler: handleListStreams,
	})

	server.RegisterTool("get_stream", Tool{
		Name:        "get_stream",
		Description: "Get detailed information about a specific stream",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Stream name",
				},
			},
			"required": []string{"name"},
		},
		Handler: handleGetStream,
	})

	server.RegisterTool("add_stream", Tool{
		Name:        "add_stream",
		Description: "Add or update a stream in go2rtc",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Stream name",
				},
				"url": map[string]any{
					"type":        "string",
					"description": "Stream source URL (e.g., rtsp://..., http://...)",
				},
			},
			"required": []string{"name", "url"},
		},
		Handler: handleAddStream,
	})

	server.RegisterTool("delete_stream", Tool{
		Name:        "delete_stream",
		Description: "Delete a stream from go2rtc",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Stream name to delete",
				},
			},
			"required": []string{"name"},
		},
		Handler: handleDeleteStream,
	})

	server.RegisterTool("get_config", Tool{
		Name:        "get_config",
		Description: "Get current go2rtc configuration",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"section": map[string]any{
					"type":        "string",
					"description": "Optional config section (e.g., streams, api)",
				},
			},
		},
		Handler: handleGetConfig,
	})

	server.RegisterTool("get_info", Tool{
		Name:        "get_info",
		Description: "Get go2rtc server information (version, host, etc.)",
		InputSchema: map[string]any{
			"type": "object",
		},
		Handler: handleGetInfo,
	})

	server.RegisterTool("list_schemes", Tool{
		Name:        "list_schemes",
		Description: "List all supported input/output schemes in go2rtc",
		InputSchema: map[string]any{
			"type": "object",
		},
		Handler: handleListSchemes,
	})

	server.RegisterTool("validate_source", Tool{
		Name:        "validate_source",
		Description: "Validate source URL and check if its scheme is supported by go2rtc",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{
					"type":        "string",
					"description": "Source URL or expression (e.g. rtsp://..., ffmpeg:..., expr:...)",
				},
			},
			"required": []string{"source"},
		},
		Handler: handleValidateSource,
	})

	server.RegisterTool("play_stream", Tool{
		Name:        "play_stream",
		Description: "Push media from a source into an existing stream",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Existing stream name",
				},
				"source": map[string]any{
					"type":        "string",
					"description": "Source URL to play into the stream",
				},
			},
			"required": []string{"name", "source"},
		},
		Handler: handlePlayStream,
	})

	server.RegisterTool("publish_stream", Tool{
		Name:        "publish_stream",
		Description: "Publish an existing stream to a destination URL",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Existing stream name",
				},
				"destination": map[string]any{
					"type":        "string",
					"description": "Destination URL for publishing (e.g. rtsp://..., rtmp://...)",
				},
			},
			"required": []string{"name", "destination"},
		},
		Handler: handlePublishStream,
	})

	server.RegisterTool("list_preloads", Tool{
		Name:        "list_preloads",
		Description: "List all preload sessions configured in go2rtc",
		InputSchema: map[string]any{
			"type": "object",
		},
		Handler: handleListPreloads,
	})

	server.RegisterTool("add_preload", Tool{
		Name:        "add_preload",
		Description: "Create or update a preload session for a stream",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Stream name",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Optional preload query (e.g. video&audio)",
				},
			},
			"required": []string{"name"},
		},
		Handler: handleAddPreload,
	})

	server.RegisterTool("delete_preload", Tool{
		Name:        "delete_preload",
		Description: "Delete an existing preload session",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Stream name with preload",
				},
			},
			"required": []string{"name"},
		},
		Handler: handleDeletePreload,
	})

	server.RegisterTool("get_streams_dot", Tool{
		Name:        "get_streams_dot",
		Description: "Build graphviz DOT topology for all streams or for a specific stream",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Optional stream name. If empty, returns graph for all streams",
				},
			},
		},
		Handler: handleGetStreamsDOT,
	})

	// Extended tools
	server.RegisterTool("get_stream_urls", Tool{
		Name:        "get_stream_urls",
		Description: "Get all available output URLs for a stream (HLS, WebRTC, MSE, RTSP, etc.)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Stream name",
				},
				"scheme": map[string]any{
					"type":        "string",
					"description": "Optional: Get URL for specific scheme only (mp4, hls, webrtc, mse, rtsp, rtmp, flv, mjpeg, websocket)",
				},
			},
			"required": []string{"name"},
		},
		Handler: handleGetStreamURLs,
	})

	server.RegisterTool("get_stream_consumers", Tool{
		Name:        "get_stream_consumers",
		Description: "Get information about active consumers connected to a stream",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Stream name",
				},
			},
			"required": []string{"name"},
		},
		Handler: handleGetStreamConsumers,
	})

	server.RegisterTool("restart_stream", Tool{
		Name:        "restart_stream",
		Description: "Restart a stream by stopping all producers and restarting them",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Stream name to restart",
				},
			},
			"required": []string{"name"},
		},
		Handler: handleRestartStream,
	})

	server.RegisterTool("get_events", Tool{
		Name:        "get_events",
		Description: "Get recent events from the event log (stream changes, connections, etc.)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{
					"type":        "number",
					"description": "Number of events to return (default: 10)",
				},
			},
		},
		Handler: handleGetEvents,
	})

	server.RegisterTool("get_connections", Tool{
		Name:        "get_connections",
		Description: "Get summary of all stream connections (producers and consumers count)",
		InputSchema: map[string]any{
			"type": "object",
		},
		Handler: handleGetConnections,
	})

	server.RegisterTool("get_stream_stats", Tool{
		Name:        "get_stream_stats",
		Description: "Get detailed statistics for a specific stream including producer states",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Stream name",
				},
			},
			"required": []string{"name"},
		},
		Handler: handleGetStreamStats,
	})

	server.RegisterTool("stream_snapshot", Tool{
		Name:        "stream_snapshot",
		Description: "Get a complete snapshot of a stream's current state",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Stream name",
				},
			},
			"required": []string{"name"},
		},
		Handler: handleStreamSnapshot,
	})

	// Register MCP resources
	server.RegisterResource("streams://", Resource{
		URI:         "streams://",
		Name:        "All Streams",
		Description: "List of all configured streams",
		MIMEType:    "application/json",
		Handler:     handleStreamsResource,
	})

	server.RegisterResource("streams://{name}", Resource{
		URI:         "streams://{name}",
		Name:        "Stream Details",
		Description: "Detailed information about a specific stream",
		MIMEType:    "application/json",
		Handler:     handleStreamResource,
	})

	server.RegisterResource("log://", Resource{
		URI:         "log://",
		Name:        "Server Log",
		Description: "Current server log",
		MIMEType:    "application/jsonlines",
		Handler:     handleLogResource,
	})

	server.RegisterResource("events://", Resource{
		URI:         "events://",
		Name:        "Event Log",
		Description: "Recent events from the event log",
		MIMEType:    "application/json",
		Handler:     handleEventsResource,
	})

	// Register MCP prompts
	server.RegisterPrompt("stream_analyzer", Prompt{
		Name:        "Stream Analyzer",
		Description: "Analyze a go2rtc stream for issues",
		Arguments: []PromptArgument{
			{
				Name:        "name",
				Description: "Stream name to analyze",
				Required:    true,
			},
		},
		Handler: handleStreamAnalyzerPrompt,
	})

	server.RegisterPrompt("stream_setup_wizard", Prompt{
		Name:        "Stream Setup Wizard",
		Description: "Generate step-by-step setup plan for a new go2rtc stream",
		Arguments: []PromptArgument{
			{
				Name:        "name",
				Description: "Stream name",
				Required:    true,
			},
			{
				Name:        "source",
				Description: "Stream source URL/expression",
				Required:    true,
			},
		},
		Handler: handleStreamSetupPrompt,
	})

	server.RegisterPrompt("operations_playbook", Prompt{
		Name:        "Operations Playbook",
		Description: "Generate maintenance/troubleshooting playbook for go2rtc",
		Arguments: []PromptArgument{
			{
				Name:        "name",
				Description: "Optional stream name to focus on",
				Required:    false,
			},
		},
		Handler: handleOperationsPlaybookPrompt,
	})

	server.RegisterPrompt("source_configurator", Prompt{
		Name:        "Source Configurator",
		Description: "Get configuration guide for specific source types (RTSP, HTTP, RTMP, FFmpeg, WebRTC, etc.)",
		Arguments: []PromptArgument{
			{
				Name:        "source_type",
				Description: "Source type: rtsp, http, rtmp, ffmpeg, webrtc, whip, whep",
				Required:    false,
			},
		},
		Handler: handleSourceConfiguratorPrompt,
	})

	server.RegisterPrompt("troubleshooting_guide", Prompt{
		Name:        "Troubleshooting Guide",
		Description: "Get step-by-step troubleshooting instructions for common issues",
		Arguments: []PromptArgument{
			{
				Name:        "issue",
				Description: "Issue type: no_connection, no_audio, no_video, high_latency, high_cpu, performance",
				Required:    false,
			},
		},
		Handler: handleTroubleshootingPrompt,
	})

	server.RegisterPrompt("integration_helper", Prompt{
		Name:        "Integration Helper",
		Description: "Get integration guides for Home Assistant, Frigate, HomeBridge, MQTT, NVRs, etc.",
		Arguments: []PromptArgument{
			{
				Name:        "platform",
				Description: "Target platform: home_assistant, frigate, homebridge, mqtt, nvr, ispy, blueiris",
				Required:    false,
			},
		},
		Handler: handleIntegrationHelperPrompt,
	})

	server.RegisterPrompt("performance_optimizer", Prompt{
		Name:        "Performance Optimizer",
		Description: "Get optimization recommendations for low latency, bandwidth, CPU usage, or reliability",
		Arguments: []PromptArgument{
			{
				Name:        "goal",
				Description: "Optimization goal: low_latency, bandwidth, cpu, reliability",
				Required:    false,
			},
		},
		Handler: handlePerformanceOptimizerPrompt,
	})

	server.RegisterPrompt("security_auditor", Prompt{
		Name:        "Security Auditor",
		Description: "Generate security checklist and audit go2rtc configuration for security best practices",
		Arguments:   []PromptArgument{},
		Handler:     handleSecurityAuditorPrompt,
	})

	server.RegisterPrompt("backup_restore", Prompt{
		Name:        "Backup & Restore Guide",
		Description: "Get instructions for backing up and restoring go2rtc configuration",
		Arguments:   []PromptArgument{},
		Handler:     handleBackupRestorePrompt,
	})

	server.RegisterPrompt("migration_helper", Prompt{
		Name:        "Migration Helper",
		Description: "Get migration guides from direct RTSP/FFmpeg/MQTT to go2rtc",
		Arguments: []PromptArgument{
			{
				Name:        "platform",
				Description: "Source platform: rtsp2mp4, ffmpeg, mqtt2go2rtc",
				Required:    false,
			},
		},
		Handler: handleMigrationPrompt,
	})

	// Start transport based on configuration
	if cfg.Mod.WebSocket {
		InitWebSocketWithFallback()
	} else if cfg.Mod.SSE {
		InitHTTPWithSSE()
	} else if cfg.Mod.HTTP == nil || *cfg.Mod.HTTP {
		InitHTTP()
	}

	if cfg.Mod.Unix != "" {
		go runUnixTransport(cfg.Mod.Unix)
	}

	// Start event notifier in background
	go StreamEventNotifier(context.Background())

	log.Info().Msg("[mcp] module initialized")
}

var log zerolog.Logger
var server *Server

// MCP Protocol Types

type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *ErrorObject    `json:"error,omitempty"`
}

type ErrorObject struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// Server represents an MCP server

type Server struct {
	tools     map[string]Tool
	resources map[string]Resource
	prompts   map[string]Prompt
	mu        sync.RWMutex
}

func NewServer() *Server {
	return &Server{
		tools:     make(map[string]Tool),
		resources: make(map[string]Resource),
		prompts:   make(map[string]Prompt),
	}
}

// Tool represents an MCP tool

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     func(ctx context.Context, args map[string]any) (any, error)
}

// Resource represents an MCP resource

type Resource struct {
	URI         string
	Name        string
	Description string
	MIMEType    string
	Handler     func(ctx context.Context, uri string) (string, error)
}

// Prompt represents an MCP prompt

type Prompt struct {
	Name        string
	Description string
	Arguments   []PromptArgument
	Handler     func(ctx context.Context, args map[string]any) ([]PromptMessage, error)
}

type PromptArgument struct {
	Name        string
	Description string
	Required    bool
}

type PromptMessage struct {
	Role    string `json:"role"`
	Content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func (s *Server) RegisterTool(name string, tool Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[name] = tool
}

func (s *Server) RegisterResource(uri string, resource Resource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[uri] = resource
}

func (s *Server) RegisterPrompt(name string, prompt Prompt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts[name] = prompt
}

func normalizePromptName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

func (s *Server) findPrompt(name string) (Prompt, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if prompt, ok := s.prompts[name]; ok {
		return prompt, true
	}

	normalized := normalizePromptName(name)
	for key, prompt := range s.prompts {
		if normalizePromptName(key) == normalized || normalizePromptName(prompt.Name) == normalized {
			return prompt, true
		}
	}

	return Prompt{}, false
}

func (s *Server) HandleMessage(msg *Message) *Message {
	ctx := context.Background()

	switch msg.Method {
	case "initialize":
		return s.handleInitialize(msg)
	case "ping":
		return &Message{JSONRPC: "2.0", ID: msg.ID, Result: map[string]any{}}
	case "notifications/initialized":
		return &Message{JSONRPC: "2.0", ID: msg.ID, Result: map[string]any{}}
	case "notifications/cancelled", "notifications/progress":
		return nil
	case "tools/list":
		return s.handleToolsList(msg)
	case "tools/call":
		return s.handleToolsCall(ctx, msg)
	case "resources/list":
		return s.handleResourcesList(msg)
	case "resources/templates/list", "resourceTemplates/list":
		return s.handleResourceTemplatesList(msg)
	case "resources/read":
		return s.handleResourcesRead(ctx, msg)
	case "prompts/list":
		return s.handlePromptsList(msg)
	case "prompts/get":
		return s.handlePromptsGet(ctx, msg)
	default:
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error: &ErrorObject{
				Code:    MethodNotFound,
				Message: fmt.Sprintf("method not found: %s", msg.Method),
			},
		}
	}
}

func (s *Server) handleInitialize(msg *Message) *Message {
	capabilities := map[string]any{
		"tools":     map[string]any{},
		"resources": map[string]any{},
		"prompts":   map[string]any{},
	}

	serverInfo := map[string]any{
		"name":    "go2rtc",
		"version": app.Info["version"],
	}

	return &Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    capabilities,
			"serverInfo":      serverInfo,
		},
	}
}

func (s *Server) handleToolsList(msg *Message) *Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]map[string]any, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": tool.InputSchema,
		})
	}

	return &Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]any{
			"tools": tools,
		},
	}
}

func (s *Server) handleToolsCall(ctx context.Context, msg *Message) *Message {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments,omitempty"`
	}

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error: &ErrorObject{
				Code:    InvalidParams,
				Message: err.Error(),
			},
		}
	}

	s.mu.RLock()
	tool, ok := s.tools[params.Name]
	s.mu.RUnlock()

	if !ok {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error: &ErrorObject{
				Code:    MethodNotFound,
				Message: fmt.Sprintf("tool not found: %s", params.Name),
			},
		}
	}

	result, err := tool.Handler(ctx, params.Arguments)
	if err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error: &ErrorObject{
				Code:    InternalError,
				Message: err.Error(),
			},
		}
	}

	return &Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]any{
			"content": []any{map[string]any{
				"type": "text",
				"text": formatResult(result),
			}},
		},
	}
}

func (s *Server) handleResourcesList(msg *Message) *Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resources := make([]map[string]any, 0, len(s.resources))
	for _, res := range s.resources {
		resources = append(resources, map[string]any{
			"uri":         res.URI,
			"name":        res.Name,
			"description": res.Description,
			"mimeType":    res.MIMEType,
		})
	}

	return &Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]any{
			"resources": resources,
		},
	}
}

func (s *Server) handleResourceTemplatesList(msg *Message) *Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	templates := make([]map[string]any, 0, len(s.resources))
	for _, res := range s.resources {
		if !strings.Contains(res.URI, "{") || !strings.Contains(res.URI, "}") {
			continue
		}

		templates = append(templates, map[string]any{
			"uriTemplate": res.URI,
			"name":        res.Name,
			"description": res.Description,
			"mimeType":    res.MIMEType,
		})
	}

	return &Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]any{
			"resourceTemplates": templates,
		},
	}
}

func (s *Server) handleResourcesRead(ctx context.Context, msg *Message) *Message {
	var params struct {
		URI string `json:"uri"`
	}

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error: &ErrorObject{
				Code:    InvalidParams,
				Message: err.Error(),
			},
		}
	}

	// Find matching resource handler
	s.mu.RLock()
	var handler func(context.Context, string) (string, error)
	var mimeType string

	for _, res := range s.resources {
		if res.URI == params.URI || strings.HasPrefix(params.URI, strings.TrimSuffix(res.URI, "//")) {
			handler = res.Handler
			mimeType = res.MIMEType
			break
		}
	}
	s.mu.RUnlock()

	if handler == nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error: &ErrorObject{
				Code:    MethodNotFound,
				Message: fmt.Sprintf("resource not found: %s", params.URI),
			},
		}
	}

	content, err := handler(ctx, params.URI)
	if err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error: &ErrorObject{
				Code:    InternalError,
				Message: err.Error(),
			},
		}
	}

	return &Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]any{
			"contents": []any{map[string]any{
				"uri":      params.URI,
				"mimeType": mimeType,
				"text":     content,
			}},
		},
	}
}

func (s *Server) handlePromptsList(msg *Message) *Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prompts := make([]map[string]any, 0, len(s.prompts))
	for _, prompt := range s.prompts {
		args := make([]map[string]any, len(prompt.Arguments))
		for i, arg := range prompt.Arguments {
			args[i] = map[string]any{
				"name":        arg.Name,
				"description": arg.Description,
				"required":    arg.Required,
			}
		}

		prompts = append(prompts, map[string]any{
			"name":        prompt.Name,
			"description": prompt.Description,
			"arguments":   args,
		})
	}

	return &Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]any{
			"prompts": prompts,
		},
	}
}

func (s *Server) handlePromptsGet(ctx context.Context, msg *Message) *Message {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments,omitempty"`
	}

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error: &ErrorObject{
				Code:    InvalidParams,
				Message: err.Error(),
			},
		}
	}

	prompt, ok := s.findPrompt(params.Name)
	if !ok {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error: &ErrorObject{
				Code:    MethodNotFound,
				Message: fmt.Sprintf("prompt not found: %s", params.Name),
			},
		}
	}

	messages, err := prompt.Handler(ctx, params.Arguments)
	if err != nil {
		return &Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error: &ErrorObject{
				Code:    InternalError,
				Message: err.Error(),
			},
		}
	}

	return &Message{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]any{
			"messages": messages,
		},
	}
}

func formatResult(result any) string {
	if b, err := json.MarshalIndent(result, "", "  "); err == nil {
		return string(b)
	}
	return fmt.Sprintf("%v", result)
}

// Transport implementations

func runUnixTransport(socketPath string) {
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Error().Err(err).Str("path", socketPath).Msg("[mcp] failed to listen on unix socket")
		return
	}

	log.Info().Str("path", socketPath).Msg("[mcp] unix transport enabled")

	defer func() {
		_ = ln.Close()
		// Remove socket file on shutdown
		_ = os.Remove(socketPath)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Error().Err(err).Msg("[mcp] unix accept error")
			continue
		}

		go handleUnixConnection(conn)
	}
}

func handleUnixConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	decoder := json.NewDecoder(reader)
	encoder := json.NewEncoder(conn)

	for {
		var msg Message
		if err := decoder.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			log.Debug().Err(err).Msg("[mcp] unix decode error")
			return
		}

		log.Trace().Str("remote", conn.RemoteAddr().String()).Msgf("[mcp] unix recv: %s", msg.Method)

		response := server.HandleMessage(&msg)

		if response != nil {
			if err := encoder.Encode(response); err != nil {
				log.Error().Err(err).Msg("[mcp] unix encode error")
				return
			}
			writer.WriteByte('\n')
			writer.Flush()

			log.Trace().Str("remote", conn.RemoteAddr().String()).Msgf("[mcp] unix send: response")
		}
	}
}

func sendError(writer *bufio.Writer, id any, code int, message string) {
	msg := &Message{
		JSONRPC: "2.0",
		ID:      id,
		Error: &ErrorObject{
			Code:    code,
			Message: message,
		},
	}
	responseJSON, _ := json.Marshal(msg)
	writer.Write(responseJSON)
	writer.WriteByte('\n')
	writer.Flush()
}
