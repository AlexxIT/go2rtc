package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/app"
)

func init() {
	// Initialize app for testing
	app.Info = map[string]any{
		"version": "1.9.14-test",
	}
}

func TestServerHandleMessage(t *testing.T) {
	s := NewServer()

	// Register test tool
	s.RegisterTool("test_tool", Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"value": map[string]any{
					"type": "string",
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			return map[string]any{"result": "ok", "value": args["value"]}, nil
		},
	})

	t.Run("initialize", func(t *testing.T) {
		msg := &Message{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "initialize",
			Params:  json.RawMessage(`{"capabilities":{}}`),
		}

		resp := s.HandleMessage(msg)
		if resp == nil {
			t.Fatal("expected response, got nil")
		}
		if resp.Error != nil {
			t.Fatalf("expected no error, got %v", resp.Error)
		}

		result, ok := resp.Result.(map[string]any)
		if !ok {
			t.Fatal("expected map result")
		}

		if protocol, ok := result["protocolVersion"].(string); !ok || protocol != "2024-11-05" {
			t.Errorf("expected protocol version 2024-11-05, got %v", result["protocolVersion"])
		}
	})

	t.Run("ping", func(t *testing.T) {
		msg := &Message{
			JSONRPC: "2.0",
			ID:      100,
			Method:  "ping",
		}

		resp := s.HandleMessage(msg)
		if resp == nil {
			t.Fatal("expected response, got nil")
		}
		if resp.Error != nil {
			t.Fatalf("expected no error, got %v", resp.Error)
		}
	})

	t.Run("cancelled notification", func(t *testing.T) {
		msg := &Message{
			JSONRPC: "2.0",
			Method:  "notifications/cancelled",
			Params:  json.RawMessage(`{"requestId":1,"reason":"test"}`),
		}

		resp := s.HandleMessage(msg)
		if resp != nil {
			t.Fatalf("expected nil response for notification, got %#v", resp)
		}
	})

	t.Run("tools/list", func(t *testing.T) {
		msg := &Message{
			JSONRPC: "2.0",
			ID:      2,
			Method:  "tools/list",
		}

		resp := s.HandleMessage(msg)
		if resp == nil || resp.Error != nil {
			t.Fatalf("expected success, got error: %v", resp)
		}

		result := resp.Result.(map[string]any)

		// Use JSON round-trip to normalize types
		resultJSON, _ := json.Marshal(result)
		var normalized struct {
			Tools []map[string]any `json:"tools"`
		}
		if err := json.Unmarshal(resultJSON, &normalized); err != nil {
			t.Fatalf("failed to normalize result: %v", err)
		}

		if len(normalized.Tools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(normalized.Tools))
		}

		if normalized.Tools[0]["name"] != "test_tool" {
			t.Errorf("expected tool name 'test_tool', got %v", normalized.Tools[0]["name"])
		}
	})

	t.Run("tools/call", func(t *testing.T) {
		params := map[string]any{
			"name": "test_tool",
			"arguments": map[string]any{
				"value": "test_value",
			},
		}
		paramsJSON, _ := json.Marshal(params)

		msg := &Message{
			JSONRPC: "2.0",
			ID:      3,
			Method:  "tools/call",
			Params:  json.RawMessage(paramsJSON),
		}

		resp := s.HandleMessage(msg)
		if resp == nil || resp.Error != nil {
			t.Fatalf("expected success, got error: %v", resp)
		}

		result := resp.Result.(map[string]any)
		content := result["content"].([]any)
		if len(content) == 0 {
			t.Fatal("expected content in result")
		}

		textContent := content[0].(map[string]any)
		if textContent["type"] != "text" {
			t.Errorf("expected text content type, got %v", textContent["type"])
		}
	})

	t.Run("method not found", func(t *testing.T) {
		msg := &Message{
			JSONRPC: "2.0",
			ID:      4,
			Method:  "unknown/method",
		}

		resp := s.HandleMessage(msg)
		if resp == nil {
			t.Fatal("expected response")
		}
		if resp.Error == nil {
			t.Fatal("expected error for unknown method")
		}
		if resp.Error.Code != MethodNotFound {
			t.Errorf("expected MethodNotFound error, got %d", resp.Error.Code)
		}
	})

	t.Run("invalid params", func(t *testing.T) {
		msg := &Message{
			JSONRPC: "2.0",
			ID:      5,
			Method:  "tools/call",
			Params:  json.RawMessage(`invalid json`),
		}

		resp := s.HandleMessage(msg)
		if resp == nil {
			t.Fatal("expected response")
		}
		if resp.Error == nil {
			t.Fatal("expected error for invalid params")
		}
		if resp.Error.Code != InvalidParams {
			t.Errorf("expected InvalidParams error, got %d", resp.Error.Code)
		}
	})
}

func TestHTTPHandler(t *testing.T) {
	// Create a test server instance
	server = NewServer()
	server.RegisterTool("test", Tool{
		Name:        "test",
		Description: "test",
		InputSchema: map[string]any{},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			return "ok", nil
		},
	})

	t.Run("POST request", func(t *testing.T) {
		reqBody := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params":  map[string]any{},
		}
		bodyJSON, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(bodyJSON))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mcpHTTPHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		ct := w.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected content-type application/json, got %s", ct)
		}

		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp["jsonrpc"] != "2.0" {
			t.Errorf("expected jsonrpc 2.0, got %v", resp["jsonrpc"])
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		w := httptest.NewRecorder()

		mcpHTTPHandler(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected status 405, got %d", w.Code)
		}
	})

	t.Run("unsupported media type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("test"))
		req.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()

		mcpHTTPHandler(w, req)

		if w.Code != http.StatusUnsupportedMediaType {
			t.Errorf("expected status 415, got %d", w.Code)
		}
	})

	t.Run("batch request", func(t *testing.T) {
		reqBody := []map[string]any{
			{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "tools/list",
			},
			{
				"jsonrpc": "2.0",
				"id":      2,
				"method":  "resources/list",
			},
		}
		bodyJSON, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(bodyJSON))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mcpHTTPHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(resp) != 2 {
			t.Errorf("expected 2 responses, got %d", len(resp))
		}
	})

	t.Run("sse post with session", func(t *testing.T) {
		session := &sseSession{
			ID:       "test-session",
			Messages: make(chan []byte, 4),
		}
		registerSSESession(session)
		defer unregisterSSESession(session.ID)

		reqBody := map[string]any{
			"jsonrpc": "2.0",
			"id":      11,
			"method":  "initialize",
			"params":  map[string]any{},
		}
		bodyJSON, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/mcp/sse?sessionId="+session.ID, bytes.NewReader(bodyJSON))
		w := httptest.NewRecorder()

		handleSSEPost(w, req)

		if w.Code != http.StatusAccepted {
			t.Fatalf("expected status 202, got %d", w.Code)
		}

		select {
		case payload := <-session.Messages:
			var resp Message
			if err := json.Unmarshal(payload, &resp); err != nil {
				t.Fatalf("failed to decode SSE payload: %v", err)
			}
			if resp.ID != float64(11) && resp.ID != 11 {
				t.Fatalf("expected response id 11, got %#v", resp.ID)
			}
		default:
			t.Fatal("expected message queued to SSE session")
		}
	})

	t.Run("sse post invalid session", func(t *testing.T) {
		reqBody := map[string]any{
			"jsonrpc": "2.0",
			"id":      12,
			"method":  "initialize",
			"params":  map[string]any{},
		}
		bodyJSON, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/mcp/sse?sessionId=missing", bytes.NewReader(bodyJSON))
		w := httptest.NewRecorder()

		handleSSEPost(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})
}

func TestResources(t *testing.T) {
	s := NewServer()

	s.RegisterResource("test://", Resource{
		URI:         "test://",
		Name:        "Test Resource",
		Description: "A test resource",
		MIMEType:    "text/plain",
		Handler: func(ctx context.Context, uri string) (string, error) {
			return "test content", nil
		},
	})
	s.RegisterResource("test://{name}", Resource{
		URI:         "test://{name}",
		Name:        "Test Template",
		Description: "A test template resource",
		MIMEType:    "text/plain",
		Handler: func(ctx context.Context, uri string) (string, error) {
			return "test template content", nil
		},
	})

	t.Run("resources/list", func(t *testing.T) {
		msg := &Message{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "resources/list",
		}

		resp := s.HandleMessage(msg)
		if resp == nil || resp.Error != nil {
			t.Fatalf("expected success, got error: %v", resp)
		}

		// Verify response has expected structure
		resultJSON, _ := json.Marshal(resp.Result)
		var normalized struct {
			Resources []map[string]any `json:"resources"`
		}
		if err := json.Unmarshal(resultJSON, &normalized); err != nil {
			t.Fatalf("failed to normalize result: %v", err)
		}

		if len(normalized.Resources) < 1 {
			t.Errorf("expected at least 1 resource, got %d", len(normalized.Resources))
		}

		var found bool
		for _, res := range normalized.Resources {
			if res["uri"] == "test://" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected resource uri test://, got %#v", normalized.Resources)
		}
	})

	t.Run("resources/read", func(t *testing.T) {
		params := map[string]any{
			"uri": "test://",
		}
		paramsJSON, _ := json.Marshal(params)

		msg := &Message{
			JSONRPC: "2.0",
			ID:      2,
			Method:  "resources/read",
			Params:  json.RawMessage(paramsJSON),
		}

		resp := s.HandleMessage(msg)
		if resp == nil || resp.Error != nil {
			t.Fatalf("expected success, got error: %v", resp)
		}

		// Verify response has expected structure
		resultJSON, _ := json.Marshal(resp.Result)
		var normalized struct {
			Contents []map[string]any `json:"contents"`
		}
		if err := json.Unmarshal(resultJSON, &normalized); err != nil {
			t.Fatalf("failed to normalize result: %v", err)
		}

		if len(normalized.Contents) == 0 {
			t.Fatal("expected contents in result")
		}

		if normalized.Contents[0]["text"] != "test content" {
			t.Errorf("expected 'test content', got %v", normalized.Contents[0]["text"])
		}
	})

	t.Run("resources/templates/list", func(t *testing.T) {
		msg := &Message{
			JSONRPC: "2.0",
			ID:      3,
			Method:  "resources/templates/list",
		}

		resp := s.HandleMessage(msg)
		if resp == nil || resp.Error != nil {
			t.Fatalf("expected success, got error: %v", resp)
		}

		resultJSON, _ := json.Marshal(resp.Result)
		var normalized struct {
			ResourceTemplates []map[string]any `json:"resourceTemplates"`
		}
		if err := json.Unmarshal(resultJSON, &normalized); err != nil {
			t.Fatalf("failed to normalize result: %v", err)
		}

		if len(normalized.ResourceTemplates) != 1 {
			t.Fatalf("expected 1 resource template, got %d", len(normalized.ResourceTemplates))
		}
		if normalized.ResourceTemplates[0]["uriTemplate"] != "test://{name}" {
			t.Fatalf("expected uriTemplate test://{name}, got %v", normalized.ResourceTemplates[0]["uriTemplate"])
		}
	})
}

func TestPrompts(t *testing.T) {
	s := NewServer()

	s.RegisterPrompt("test_prompt", Prompt{
		Name:        "test_prompt",
		Description: "A test prompt",
		Arguments: []PromptArgument{
			{
				Name:        "arg1",
				Description: "First argument",
				Required:    true,
			},
		},
		Handler: func(ctx context.Context, args map[string]any) ([]PromptMessage, error) {
			return []PromptMessage{
				{
					Role: "user",
				},
			}, nil
		},
	})

	t.Run("prompts/list", func(t *testing.T) {
		msg := &Message{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "prompts/list",
		}

		resp := s.HandleMessage(msg)
		if resp == nil || resp.Error != nil {
			t.Fatalf("expected success, got error: %v", resp)
		}

		// Verify response has expected structure
		resultJSON, _ := json.Marshal(resp.Result)
		var normalized struct {
			Prompts []map[string]any `json:"prompts"`
		}
		if err := json.Unmarshal(resultJSON, &normalized); err != nil {
			t.Fatalf("failed to normalize result: %v", err)
		}

		if len(normalized.Prompts) != 1 {
			t.Errorf("expected 1 prompt, got %d", len(normalized.Prompts))
		}

		if normalized.Prompts[0]["name"] != "test_prompt" {
			t.Errorf("expected prompt name 'test_prompt', got %v", normalized.Prompts[0]["name"])
		}
	})

	t.Run("prompts/get", func(t *testing.T) {
		params := map[string]any{
			"name": "test_prompt",
			"arguments": map[string]any{
				"arg1": "value1",
			},
		}
		paramsJSON, _ := json.Marshal(params)

		msg := &Message{
			JSONRPC: "2.0",
			ID:      2,
			Method:  "prompts/get",
			Params:  json.RawMessage(paramsJSON),
		}

		resp := s.HandleMessage(msg)
		if resp == nil || resp.Error != nil {
			t.Fatalf("expected success, got error: %v", resp)
		}

		// Verify response has expected structure
		resultJSON, _ := json.Marshal(resp.Result)
		var normalized struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.Unmarshal(resultJSON, &normalized); err != nil {
			t.Fatalf("failed to normalize result: %v", err)
		}

		if len(normalized.Messages) == 0 {
			t.Fatal("expected messages in result")
		}
	})

	t.Run("prompts/get by display name", func(t *testing.T) {
		s.RegisterPrompt("integration_helper", Prompt{
			Name:        "Integration Helper",
			Description: "A test prompt with display name",
			Handler: func(ctx context.Context, args map[string]any) ([]PromptMessage, error) {
				return []PromptMessage{
					{
						Role: "user",
					},
				}, nil
			},
		})

		params := map[string]any{
			"name": "Integration Helper",
		}
		paramsJSON, _ := json.Marshal(params)

		msg := &Message{
			JSONRPC: "2.0",
			ID:      3,
			Method:  "prompts/get",
			Params:  json.RawMessage(paramsJSON),
		}

		resp := s.HandleMessage(msg)
		if resp == nil || resp.Error != nil {
			t.Fatalf("expected success, got error: %v", resp)
		}
	})
}

func TestStreamHandlers(t *testing.T) {
	// This test requires streams module to be initialized
	// We'll test the handler functions in isolation

	t.Run("handleGetInfo", func(t *testing.T) {
		app.Info["test"] = "value"
		result, err := handleGetInfo(context.Background(), map[string]any{})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		info, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map result")
		}

		if info["test"] != "value" {
			t.Errorf("expected test value, got %v", info["test"])
		}
	})

	t.Run("handleGetConfig", func(t *testing.T) {
		result, err := handleGetConfig(context.Background(), map[string]any{})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		config, ok := result.(map[string]any)
		if !ok {
			t.Fatal("expected map result")
		}

		if config["info"] == nil {
			t.Error("expected info in config result")
		}
	})
}

func BenchmarkHandleMessage(b *testing.B) {
	s := NewServer()

	s.RegisterTool("bench_tool", Tool{
		Name:        "bench_tool",
		Description: "Benchmark tool",
		InputSchema: map[string]any{},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			return map[string]any{"status": "ok"}, nil
		},
	})

	msg := &Message{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"bench_tool","arguments":{}}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.HandleMessage(msg)
	}
}
