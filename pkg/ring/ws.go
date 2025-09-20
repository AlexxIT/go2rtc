package ring

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type SessionBody struct {
	DoorbotID int    `json:"doorbot_id"`
	SessionID string `json:"session_id"`
}

type AnswerMessage struct {
	Method string `json:"method"` // "sdp"
	Body   struct {
		SessionBody
		SDP  string `json:"sdp"`
		Type string `json:"type"` // "answer"
	} `json:"body"`
}

type IceCandidateMessage struct {
	Method string `json:"method"` // "ice"
	Body   struct {
		SessionBody
		Ice        string `json:"ice"`
		MLineIndex int    `json:"mlineindex"`
	} `json:"body"`
}

type SessionMessage struct {
	Method string      `json:"method"` // "session_created" or "session_started"
	Body   SessionBody `json:"body"`
}

type PongMessage struct {
	Method string      `json:"method"` // "pong"
	Body   SessionBody `json:"body"`
}

type NotificationMessage struct {
	Method string `json:"method"` // "notification"
	Body   struct {
		SessionBody
		IsOK bool   `json:"is_ok"`
		Text string `json:"text"`
	} `json:"body"`
}

type StreamInfoMessage struct {
	Method string `json:"method"` // "stream_info"
	Body   struct {
		SessionBody
		Transcoding       bool   `json:"transcoding"`
		TranscodingReason string `json:"transcoding_reason"`
	} `json:"body"`
}

type CloseRequest struct {
	Method string `json:"method"` // "close"
	Body   struct {
		SessionBody
		Reason struct {
			Code int    `json:"code"`
			Text string `json:"text"`
		} `json:"reason"`
	} `json:"body"`
}

type WSMessage struct {
	Method string         `json:"method"`
	Body   map[string]any `json:"body"`
}

type WSClient struct {
	ws        *websocket.Conn
	api       *RingApi
	wsMutex   sync.Mutex
	cameraID  int
	dialogID  string
	sessionID string

	onMessage func(msg WSMessage)
	onError   func(err error)
	onClose   func()

	closed chan struct{}
}

const (
	CloseReasonNormalClose          = 0
	CloseReasonAuthenticationFailed = 5
	CloseReasonTimeout              = 6
)

func StartWebsocket(cameraID int, api *RingApi) (*WSClient, error) {
	client := &WSClient{
		api:      api,
		cameraID: cameraID,
		dialogID: uuid.NewString(),
		closed:   make(chan struct{}),
	}

	ticket, err := client.api.GetSocketTicket()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("wss://api.prod.signalling.ring.devices.a2z.com/ws?api_version=4.0&auth_type=ring_solutions&client_id=ring_site-%s&token=%s",
		uuid.NewString(), url.QueryEscape(ticket.Ticket))

	httpHeader := http.Header{}
	httpHeader.Set("User-Agent", "android:com.ringapp")

	client.ws, _, err = websocket.DefaultDialer.Dial(url, httpHeader)
	if err != nil {
		return nil, err
	}

	client.ws.SetCloseHandler(func(code int, text string) error {
		client.onWsClose()
		return nil
	})

	go client.startPingLoop()
	go client.startMessageLoop()

	return client, nil
}

func (c *WSClient) Close() error {
	select {
	case <-c.closed:
		return nil
	default:
		close(c.closed)
	}

	closePayload := map[string]interface{}{
		"reason": map[string]interface{}{
			"code": CloseReasonNormalClose,
			"text": "",
		},
	}

	_ = c.sendSessionMessage("close", closePayload)

	return c.ws.Close()
}

func (c *WSClient) startPingLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.closed:
			return
		case <-ticker.C:
			if err := c.sendSessionMessage("ping", nil); err != nil {
				return
			}
		}
	}
}

func (c *WSClient) startMessageLoop() {
	for {
		select {
		case <-c.closed:
			return
		default:
			var res WSMessage
			if err := c.ws.ReadJSON(&res); err != nil {
				select {
				case <-c.closed:
					// Ignore error if closed
				default:
					c.onWsError(err)
				}

				return
			}

			c.onWsMessage(res)
		}
	}
}

func (c *WSClient) activateSession() error {
	if err := c.sendSessionMessage("activate_session", nil); err != nil {
		return err
	}

	streamPayload := map[string]interface{}{
		"audio_enabled": true,
		"video_enabled": true,
	}

	if err := c.sendSessionMessage("stream_options", streamPayload); err != nil {
		return err
	}

	return nil
}

func (c *WSClient) sendSessionMessage(method string, payload map[string]interface{}) error {
	select {
	case <-c.closed:
		return nil
	default:
		// continue
	}

	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()

	if payload == nil {
		payload = make(map[string]interface{})
	}

	payload["doorbot_id"] = c.cameraID
	if c.sessionID != "" {
		payload["session_id"] = c.sessionID
	}

	msg := map[string]interface{}{
		"method":    method,
		"dialog_id": c.dialogID,
		"body":      payload,
	}

	// rawMsg, _ := json.Marshal(msg)
	// fmt.Printf("ring: sendSessionMessage: %s: %s\n", method, string(rawMsg))

	if err := c.ws.WriteJSON(msg); err != nil {
		return err
	}

	return nil
}

func (c *WSClient) onWsMessage(msg WSMessage) {
	if c.onMessage != nil {
		c.onMessage(msg)
	}
}

func (c *WSClient) onWsError(err error) {
	if c.onError != nil {
		c.onError(err)
	}
}

func (c *WSClient) onWsClose() {
	if c.onClose != nil {
		c.onClose()
	}
}
