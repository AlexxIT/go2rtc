package ring

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v4"
)

type Client struct {
	api       *RingRestClient
	ws        *websocket.Conn
	prod      core.Producer
	camera    *CameraData
	dialogID  string
	sessionID string
	wsMutex   sync.Mutex
	done      chan struct{}
}

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

type CloseMessage struct {
	Method string `json:"method"` // "close"
	Body   struct {
		SessionBody
		Reason struct {
			Code int    `json:"code"`
			Text string `json:"text"`
		} `json:"reason"`
	} `json:"body"`
}

type BaseMessage struct {
	Method string         `json:"method"`
	Body   map[string]any `json:"body"`
}

// Close reason codes
const (
	CloseReasonNormalClose          = 0
	CloseReasonAuthenticationFailed = 5
	CloseReasonTimeout              = 6
)

func Dial(rawURL string) (*Client, error) {
	// 1. Parse URL and validate basic params
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	encodedToken := query.Get("refresh_token")
	deviceID := query.Get("device_id")
	_, isSnapshot := query["snapshot"]

	if encodedToken == "" || deviceID == "" {
		return nil, errors.New("ring: wrong query")
	}

	// URL-decode the refresh token
	refreshToken, err := url.QueryUnescape(encodedToken)
	if err != nil {
		return nil, fmt.Errorf("ring: invalid refresh token encoding: %w", err)
	}

	// Initialize Ring API client
	ringAPI, err := NewRingRestClient(RefreshTokenAuth{RefreshToken: refreshToken}, nil)
	if err != nil {
		return nil, err
	}

	// Get camera details
	devices, err := ringAPI.FetchRingDevices()
	if err != nil {
		return nil, err
	}

	var camera *CameraData
	for _, cam := range devices.AllCameras {
		if fmt.Sprint(cam.DeviceID) == deviceID {
			camera = &cam
			break
		}
	}
	if camera == nil {
		return nil, errors.New("ring: camera not found")
	}

	// Create base client
	client := &Client{
		api:      ringAPI,
		camera:   camera,
		dialogID: uuid.NewString(),
		done:     make(chan struct{}),
	}

	// Check if snapshot request
	if isSnapshot {
		client.prod = NewSnapshotProducer(ringAPI, camera)
		return client, nil
	}

	// If not snapshot, continue with WebRTC setup
	ticket, err := ringAPI.GetSocketTicket()
	if err != nil {
		return nil, err
	}

	// Create WebSocket connection
	wsURL := fmt.Sprintf("wss://api.prod.signalling.ring.devices.a2z.com/ws?api_version=4.0&auth_type=ring_solutions&client_id=ring_site-%s&token=%s",
		uuid.NewString(), url.QueryEscape(ticket.Ticket))

	client.ws, _, err = websocket.DefaultDialer.Dial(wsURL, map[string][]string{
		"User-Agent": {"android:com.ringapp"},
	})
	if err != nil {
		return nil, err
	}

	// Create Peer Connection
	conf := pion.Configuration{
		ICEServers: []pion.ICEServer{
			{URLs: []string{
				"stun:stun.kinesisvideo.us-east-1.amazonaws.com:443",
				"stun:stun.kinesisvideo.us-east-2.amazonaws.com:443",
				"stun:stun.kinesisvideo.us-west-2.amazonaws.com:443",
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
				"stun:stun2.l.google.com:19302",
				"stun:stun3.l.google.com:19302",
				"stun:stun4.l.google.com:19302",
			}},
		},
		ICETransportPolicy: pion.ICETransportPolicyAll,
		BundlePolicy:       pion.BundlePolicyBalanced,
	}

	api, err := webrtc.NewAPI()
	if err != nil {
		client.ws.Close()
		return nil, err
	}

	pc, err := api.NewPeerConnection(conf)
	if err != nil {
		client.ws.Close()
		return nil, err
	}

	// protect from sending ICE candidate before Offer
	var sendOffer core.Waiter

	// protect from blocking on errors
	defer sendOffer.Done(nil)

	// waiter will wait PC error or WS error or nil (connection OK)
	var connState core.Waiter

	prod := webrtc.NewConn(pc)
	prod.FormatName = "ring/webrtc"
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "ws"
	prod.URL = rawURL

	client.prod = prod

	prod.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *pion.ICECandidate:
			_ = sendOffer.Wait()

			iceCandidate := msg.ToJSON()

			// skip empty ICE candidates
			if iceCandidate.Candidate == "" {
				return
			}

			icePayload := map[string]interface{}{
				"ice":        iceCandidate.Candidate,
				"mlineindex": iceCandidate.SDPMLineIndex,
			}

			if err = client.sendSessionMessage("ice", icePayload); err != nil {
				connState.Done(err)
				return
			}

		case pion.PeerConnectionState:
			switch msg {
			case pion.PeerConnectionStateConnecting:
			case pion.PeerConnectionStateConnected:
				connState.Done(nil)
			default:
				connState.Done(errors.New("ring: " + msg.String()))
			}
		}
	})

	// Setup media configuration
	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendRecv,
			Codecs: []*core.Codec{
				{
					Name:      "opus",
					ClockRate: 48000,
					Channels:  2,
				},
			},
		},
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{
					Name:      "H264",
					ClockRate: 90000,
				},
			},
		},
	}

	// Create offer
	offer, err := prod.CreateOffer(medias)
	if err != nil {
		client.Stop()
		return nil, err
	}

	// Send offer
	offerPayload := map[string]interface{}{
		"stream_options": map[string]bool{
			"audio_enabled": true,
			"video_enabled": true,
		},
		"sdp": offer,
	}

	if err = client.sendSessionMessage("live_view", offerPayload); err != nil {
		client.Stop()
		return nil, err
	}

	sendOffer.Done(nil)

	// Ring expects a ping message every 5 seconds
	go client.startPingLoop(pc)
	go client.startMessageLoop(&connState)

	if err = connState.Wait(); err != nil {
		return nil, err
	}

	return client, nil
}

func (c *Client) startPingLoop(pc *pion.PeerConnection) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if pc.ConnectionState() == pion.PeerConnectionStateConnected {
				if err := c.sendSessionMessage("ping", nil); err != nil {
					return
				}
			}
		}
	}
}

func (c *Client) startMessageLoop(connState *core.Waiter) {
	var err error

	// will be closed when conn will be closed
	defer func() {
		connState.Done(err)
	}()

	for {
		select {
		case <-c.done:
			return
		default:
			var res BaseMessage
			if err = c.ws.ReadJSON(&res); err != nil {
				select {
				case <-c.done:
					return
				default:
				}

				c.Stop()
				return
			}

			// check if "doorbot_id" is present
			if _, ok := res.Body["doorbot_id"]; !ok {
				continue
			}

			// check if the message is from the correct doorbot
			doorbotID := res.Body["doorbot_id"].(float64)
			if doorbotID != float64(c.camera.ID) {
				continue
			}

			// check if the message is from the correct session
			if res.Method == "session_created" || res.Method == "session_started" {
				if _, ok := res.Body["session_id"]; ok && c.sessionID == "" {
					c.sessionID = res.Body["session_id"].(string)
				}
			}

			if _, ok := res.Body["session_id"]; ok {
				if res.Body["session_id"].(string) != c.sessionID {
					continue
				}
			}

			rawMsg, _ := json.Marshal(res)

			switch res.Method {
			case "sdp":
				if prod, ok := c.prod.(*webrtc.Conn); ok {
					// Get answer
					var msg AnswerMessage
					if err = json.Unmarshal(rawMsg, &msg); err != nil {
						c.Stop()
						return
					}
					if err = prod.SetAnswer(msg.Body.SDP); err != nil {
						c.Stop()
						return
					}
					if err = c.activateSession(); err != nil {
						c.Stop()
						return
					}
				}

			case "ice":
				if prod, ok := c.prod.(*webrtc.Conn); ok {
					// Continue to receiving candidates
					var msg IceCandidateMessage
					if err = json.Unmarshal(rawMsg, &msg); err != nil {
						break
					}

					// check for empty ICE candidate
					if msg.Body.Ice == "" {
						break
					}

					if err = prod.AddCandidate(msg.Body.Ice); err != nil {
						c.Stop()
						return
					}
				}

			case "close":
				c.Stop()
				return

			case "pong":
				// Ignore
				continue
			}
		}
	}
}

func (c *Client) activateSession() error {
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

func (c *Client) sendSessionMessage(method string, body map[string]interface{}) error {
	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()

	if body == nil {
		body = make(map[string]interface{})
	}

	body["doorbot_id"] = c.camera.ID
	if c.sessionID != "" {
		body["session_id"] = c.sessionID
	}

	msg := map[string]interface{}{
		"method":    method,
		"dialog_id": c.dialogID,
		"body":      body,
	}

	if err := c.ws.WriteJSON(msg); err != nil {
		return err
	}

	return nil
}

func (c *Client) GetMedias() []*core.Media {
	return c.prod.GetMedias()
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return c.prod.GetTrack(media, codec)
}

func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	if webrtcProd, ok := c.prod.(*webrtc.Conn); ok {
		if media.Kind == core.KindAudio {
			// Enable speaker
			speakerPayload := map[string]interface{}{
				"stealth_mode": false,
			}
			_ = c.sendSessionMessage("camera_options", speakerPayload)
		}
		return webrtcProd.AddTrack(media, codec, track)
	}

	return fmt.Errorf("add track not supported for snapshot")
}

func (c *Client) Start() error {
	return c.prod.Start()
}

func (c *Client) Stop() error {
	select {
	case <-c.done:
		return nil
	default:
		close(c.done)
	}

	if c.prod != nil {
		_ = c.prod.Stop()
	}

	if c.ws != nil {
		closePayload := map[string]interface{}{
			"reason": map[string]interface{}{
				"code": CloseReasonNormalClose,
				"text": "",
			},
		}

		_ = c.sendSessionMessage("close", closePayload)
		_ = c.ws.Close()
		c.ws = nil
	}

	return nil
}

func (c *Client) MarshalJSON() ([]byte, error) {
	if webrtcProd, ok := c.prod.(*webrtc.Conn); ok {
		return webrtcProd.MarshalJSON()
	}

	return json.Marshal(c.prod)
}
