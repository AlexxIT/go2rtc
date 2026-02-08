package mcp

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from any origin when CORS is configured
		return true
	},
}

// InitWebSocket registers the WebSocket MCP endpoint
func InitWebSocket() {
	if server == nil {
		return
	}

	api.HandleFunc("mcp/ws", mcpWebSocketHandler)
	log.Info().Msg("[mcp] websocket transport enabled")
}

// mcpWebSocketHandler handles MCP JSON-RPC requests over WebSocket
func mcpWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("[mcp] websocket upgrade failed")
		return
	}

	log.Info().Str("remote", r.RemoteAddr).Msg("[mcp] websocket connection established")

	wsSession := &wsSession{
		conn:     conn,
		messages: make(chan []byte, 256),
		closed:   make(chan struct{}),
	}

	go wsSession.writePump()
	wsSession.readPump()

	_ = conn.Close()
	close(wsSession.closed)

	log.Info().Str("remote", r.RemoteAddr).Msg("[mcp] websocket connection closed")
}

type wsSession struct {
	conn     *websocket.Conn
	messages chan []byte
	closed   chan struct{}
	mu       sync.Mutex
}

func (s *wsSession) readPump() {
	defer func() {
		select {
		case <-s.closed:
			return
		default:
			close(s.messages)
		}
	}()

	_ = s.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	s.conn.SetPongHandler(func(string) error {
		_ = s.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Msg("[mcp] websocket read error")
			}
			break
		}

		log.Trace().Str("remote", s.conn.RemoteAddr().String()).Msgf("[mcp] ws recv: %s", string(message))

		// Parse JSON-RPC message
		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			s.sendError(nil, ParseError, err.Error())
			continue
		}

		response := server.HandleMessage(&msg)

		if response != nil {
			responseJSON, _ := json.Marshal(response)
			select {
			case s.messages <- responseJSON:
			case <-s.closed:
				return
			}
		}
	}
}

func (s *wsSession) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.closed:
			return
		case message, ok := <-s.messages:
			if !ok {
				_ = s.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			s.mu.Lock()
			err := s.conn.WriteMessage(websocket.TextMessage, message)
			s.mu.Unlock()

			if err != nil {
				log.Error().Err(err).Msg("[mcp] websocket write error")
				return
			}

			log.Trace().Str("remote", s.conn.RemoteAddr().String()).Msgf("[mcp] ws send: response")

		case <-ticker.C:
			s.mu.Lock()
			if err := s.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				s.mu.Unlock()
				return
			}
			s.mu.Unlock()
		}
	}
}

func (s *wsSession) sendError(id any, code int, message string) {
	msg := &Message{
		JSONRPC: "2.0",
		ID:      id,
		Error: &ErrorObject{
			Code:    code,
			Message: message,
		},
	}
	responseJSON, _ := json.Marshal(msg)
	select {
	case s.messages <- responseJSON:
	case <-s.closed:
	}
}

// InitWebSocketWithFallback registers WebSocket with SSE fallback
func InitWebSocketWithFallback() {
	InitWebSocket()
	InitHTTPWithSSE()
	log.Info().Msg("[mcp] websocket + sse transport enabled")
}
