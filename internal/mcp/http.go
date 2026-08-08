package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
)

func InitHTTP() {
	if server == nil {
		return
	}

	// Register HTTP endpoint for MCP
	api.HandleFunc("mcp", mcpHTTPHandler)

	log.Info().Msg("[mcp] http transport enabled")
}

// mcpHTTPHandler handles MCP JSON-RPC requests over HTTP
func mcpHTTPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check content type
	contentType := r.Header.Get("Content-Type")
	if contentType != "" && contentType != "application/json" && contentType != "application/json-rpc" {
		http.Error(w, "Unsupported media type", http.StatusUnsupportedMediaType)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendHTTPError(w, nil, InvalidRequest, "failed to read request body")
		return
	}

	log.Trace().Str("remote", r.RemoteAddr).Msgf("[mcp] http recv: %s", string(body))

	// Handle both single request and batch requests
	var messages []Message
	if len(body) > 0 && body[0] == '[' {
		// Batch request
		if err := json.Unmarshal(body, &messages); err != nil {
			sendHTTPError(w, nil, ParseError, err.Error())
			return
		}
	} else {
		// Single request
		var msg Message
		if err := json.Unmarshal(body, &msg); err != nil {
			sendHTTPError(w, nil, ParseError, err.Error())
			return
		}
		messages = []Message{msg}
	}

	// Process all messages
	responses := make([]*Message, 0, len(messages))
	for _, msg := range messages {
		response := server.HandleMessage(&msg)
		if response != nil {
			responses = append(responses, response)
		}
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")

	if len(responses) == 0 {
		// Empty response for notifications
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var responseJSON []byte
	if len(responses) == 1 {
		responseJSON, _ = json.Marshal(responses[0])
	} else {
		responseJSON, _ = json.Marshal(responses)
	}

	log.Trace().Str("remote", r.RemoteAddr).Msgf("[mcp] http send: %s", string(responseJSON))

	w.WriteHeader(http.StatusOK)
	w.Write(responseJSON)
}

// mcpHTTPHandlerSSE handles MCP with Server-Sent Events support
func mcpHTTPHandlerSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set headers for SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method == http.MethodGet {
		// SSE connection with session-bound outbound messages.
		handleSSEConnection(w, r, flusher)
		return
	}

	// POST request for bidirectional messaging.
	handleSSEPost(w, r)
}

type sseSession struct {
	ID       string
	Messages chan []byte
}

var (
	sseSessions   = map[string]*sseSession{}
	sseSessionsMu sync.RWMutex
)

func handleSSEConnection(w http.ResponseWriter, r *http.Request, flusher http.Flusher) {
	sessionID := newSSESessionID()
	session := &sseSession{
		ID:       sessionID,
		Messages: make(chan []byte, 64),
	}

	registerSSESession(session)
	defer unregisterSSESession(sessionID)

	_, _ = io.WriteString(w, fmt.Sprintf("event: endpoint\ndata: /mcp/sse?sessionId=%s\n\n", sessionID))
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case payload := <-session.Messages:
			_, _ = io.WriteString(w, "event: message\n")
			_, _ = io.WriteString(w, "data: ")
			_, _ = w.Write(payload)
			_, _ = io.WriteString(w, "\n\n")
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = io.WriteString(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func handleSSEPost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendHTTPError(w, nil, InvalidRequest, "failed to read request body")
		return
	}

	sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
	if sessionID != "" {
		handleSSEPostWithSession(w, body, sessionID)
		return
	}

	// Compatibility fallback: if session is not used, behave like plain HTTP MCP.
	var messages []Message
	if len(body) > 0 && body[0] == '[' {
		if err := json.Unmarshal(body, &messages); err != nil {
			sendHTTPError(w, nil, ParseError, err.Error())
			return
		}
	} else {
		var msg Message
		if err := json.Unmarshal(body, &msg); err != nil {
			sendHTTPError(w, nil, ParseError, err.Error())
			return
		}
		messages = []Message{msg}
	}

	responses := make([]*Message, 0, len(messages))
	for _, msg := range messages {
		if response := server.HandleMessage(&msg); response != nil {
			responses = append(responses, response)
		}
	}

	if len(responses) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if len(responses) == 1 {
		responseJSON, _ := json.Marshal(responses[0])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseJSON)
		return
	}

	responseJSON, _ := json.Marshal(responses)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(responseJSON)
}

func handleSSEPostWithSession(w http.ResponseWriter, body []byte, sessionID string) {
	_, ok := getSSESession(sessionID)
	if !ok {
		sendHTTPError(w, nil, InvalidRequest, "invalid or expired SSE session")
		return
	}

	// Handle both single request and batch requests.
	var messages []Message
	if len(body) > 0 && body[0] == '[' {
		if err := json.Unmarshal(body, &messages); err != nil {
			sendHTTPError(w, nil, ParseError, err.Error())
			return
		}
	} else {
		var msg Message
		if err := json.Unmarshal(body, &msg); err != nil {
			sendHTTPError(w, nil, ParseError, err.Error())
			return
		}
		messages = []Message{msg}
	}

	for _, msg := range messages {
		response := server.HandleMessage(&msg)
		if response == nil {
			continue
		}

		payload, _ := json.Marshal(response)
		if !sendSSEMessage(sessionID, payload) {
			sendHTTPError(w, nil, InvalidRequest, "SSE session is closed")
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
}

func registerSSESession(session *sseSession) {
	sseSessionsMu.Lock()
	sseSessions[session.ID] = session
	sseSessionsMu.Unlock()
}

func unregisterSSESession(sessionID string) {
	sseSessionsMu.Lock()
	delete(sseSessions, sessionID)
	sseSessionsMu.Unlock()
}

func getSSESession(sessionID string) (*sseSession, bool) {
	sseSessionsMu.RLock()
	session, ok := sseSessions[sessionID]
	sseSessionsMu.RUnlock()
	return session, ok
}

func sendSSEMessage(sessionID string, payload []byte) bool {
	session, ok := getSSESession(sessionID)
	if !ok {
		return false
	}

	select {
	case session.Messages <- payload:
		return true
	default:
		return false
	}
}

func newSSESessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("sse-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func sendHTTPError(w http.ResponseWriter, id any, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      id,
		Error: &ErrorObject{
			Code:    code,
			Message: message,
		},
	}
	responseJSON, _ := json.Marshal(msg)
	w.Write(responseJSON)
}

// InitHTTPWithSSE registers both regular HTTP and SSE endpoints
func InitHTTPWithSSE() {
	InitHTTP()
	api.HandleFunc("mcp/sse", mcpHTTPHandlerSSE)
	log.Info().Msg("[mcp] http sse transport enabled")
}
