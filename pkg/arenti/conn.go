package arenti

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v4"
)

type Conn struct {
	*webrtc.Conn

	client    *Client
	dev       *Device
	isBattery bool
	wsConn    *websocket.Conn
	sessionID string
	callerID  string
	callee    string

	ticker    *time.Ticker
	done      chan struct{}
	closeOnce sync.Once
}

var Log = func(format string, a ...any) {}

func newRtcAPI() (*pion.API, error) {
	m := &pion.MediaEngine{}
	if err := m.RegisterCodec(pion.RTPCodecParameters{
		RTPCodecCapability: pion.RTPCodecCapability{
			MimeType:    pion.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
		PayloadType: 96,
	}, pion.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	if err := m.RegisterCodec(pion.RTPCodecParameters{
		RTPCodecCapability: pion.RTPCodecCapability{
			MimeType:  pion.MimeTypePCMU,
			ClockRate: 8000,
		},
		PayloadType: 0,
	}, pion.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	if err := m.RegisterCodec(pion.RTPCodecParameters{
		RTPCodecCapability: pion.RTPCodecCapability{
			MimeType:  pion.MimeTypePCMA,
			ClockRate: 8000,
		},
		PayloadType: 8,
	}, pion.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	return pion.NewAPI(pion.WithMediaEngine(m)), nil
}

// NewProducer connects to an Arenti camera via WebRTC (MTS protocol) on-demand.
func NewProducer(client *Client, dev *Device, rawURL string) (*Conn, error) {
	callee := strings.TrimPrefix(dev.SnNum, "ppsl")
	deviceCode := dev.HostKey

	// 1. Generate unique session and caller IDs
	sessionID := NewUUID()
	callerIDBytes := make([]byte, 8)
	if _, err := rand.Read(callerIDBytes); err != nil {
		return nil, err
	}
	callerID := hex.EncodeToString(callerIDBytes)

	// 2. Obtain WebSocket signaling ticket from REST API
	expires := fmt.Sprintf("%d", time.Now().UnixMilli())
	signResp, err := client.GetSignWss(callee, deviceCode, expires)
	if err != nil {
		return nil, fmt.Errorf("arenti: failed to obtain wss sign ticket: %w", err)
	}

	// 3. Connect to Meari signaling WebSocket gateway
	wssDomain := client.WssDomain
	if wssDomain == "" {
		if strings.Contains(client.BaseURL, "web-us") {
			wssDomain = "wss://wss-us.mearicloud.com"
		} else {
			wssDomain = "wss://wss-eu.mearicloud.com"
		}
	}

	Log("arenti: connecting to wss %s (callee: %s)", wssDomain, callee)

	header := http.Header{}
	header.Set("Origin", "https://web.arenti.net")

	wsConn, _, err := websocket.DefaultDialer.Dial(wssDomain, header)
	if err != nil {
		return nil, fmt.Errorf("arenti: websocket dial failed: %w", err)
	}

	// 4. Send "hello"
	helloMsg := map[string]interface{}{
		"action": "req",
		"cmd":    "mts",
		"method": "hello",
		"sid":    sessionID,
	}
	if err := wsConn.WriteJSON(helloMsg); err != nil {
		_ = wsConn.Close()
		return nil, fmt.Errorf("arenti: failed to send hello: %w", err)
	}

	// 5. Send "option" to obtain COTURN credentials
	optionMsg := map[string]interface{}{
		"action": "req",
		"cmd":    "mts",
		"method": "option",
		"sid":    sessionID,
		"auth": map[string]string{
			"accessid":  signResp.Data.AccessID,
			"signature": signResp.Data.Signature,
			"token":     signResp.Data.Token,
		},
		"params": map[string]string{
			"caller":     callerID,
			"callee":     callee,
			"devicecode": deviceCode,
			"expires":    expires,
		},
	}
	if err := wsConn.WriteJSON(optionMsg); err != nil {
		_ = wsConn.Close()
		return nil, fmt.Errorf("arenti: failed to send option: %w", err)
	}

	// 6. Read until COTURN credentials arrive in "option" response
	_ = wsConn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var coturn TurnCredentials
	for {
		_, msgBytes, err := wsConn.ReadMessage()
		if err != nil {
			_ = wsConn.Close()
			return nil, fmt.Errorf("arenti: read option response error: %w", err)
		}

		var rsp struct {
			Action string          `json:"action"`
			Cmd    string          `json:"cmd"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(msgBytes, &rsp); err == nil && rsp.Method == "option" {
			if err := json.Unmarshal(rsp.Params, &coturn); err == nil && coturn.CoturnHost != "" {
				break
			}
		}
	}

	if coturn.CoturnHost == "" {
		_ = wsConn.Close()
		return nil, errors.New("arenti: empty COTURN host received from cloud")
	}

	Log("arenti: received coturn %s:%d", coturn.CoturnHost, coturn.CoturnPort)

	// 7. Initialize Pion PeerConnection with H264 baseline + audio
	rtcAPI, err := newRtcAPI()
	if err != nil {
		_ = wsConn.Close()
		return nil, err
	}

	iceServers := []pion.ICEServer{
		{
			URLs: []string{
				fmt.Sprintf("turn:%s:%d?transport=udp", coturn.CoturnHost, coturn.CoturnPort),
				fmt.Sprintf("stun:%s:%d", coturn.CoturnHost, coturn.CoturnPort),
			},
			Username:   coturn.Username,
			Credential: coturn.Password,
		},
	}

	pc, err := rtcAPI.NewPeerConnection(pion.Configuration{
		ICEServers: iceServers,
	})
	if err != nil {
		_ = wsConn.Close()
		return nil, err
	}

	webrtcConn := webrtc.NewConn(pc)
	webrtcConn.FormatName = "arenti/webrtc"
	webrtcConn.Mode = core.ModeActiveProducer
	webrtcConn.Protocol = "wss"
	webrtcConn.URL = rawURL

	isBattery := true
	if strings.Contains(rawURL, "battery=false") || strings.Contains(rawURL, "battery=no") || strings.Contains(rawURL, "battery=0") {
		isBattery = false
	} else if strings.Contains(rawURL, "battery=true") || strings.Contains(rawURL, "battery=yes") || strings.Contains(rawURL, "battery=1") {
		isBattery = true
	} else if client.Battery != nil {
		isBattery = *client.Battery
	} else if dev != nil && dev.Category == "ipc" && dev.Battery == 0 {
		isBattery = false
	}

	conn := &Conn{
		Conn:      webrtcConn,
		client:    client,
		dev:       dev,
		isBattery: isBattery,
		wsConn:    wsConn,
		sessionID: sessionID,
		callerID:  callerID,
		callee:    callee,
		done:      make(chan struct{}),
	}

	// Listen for ICE candidates and forward to camera via WebSocket
	webrtcConn.Listen(func(msg any) {
		if c, ok := msg.(*pion.ICECandidate); ok && c != nil {
			candJSON := c.ToJSON()
			Log("arenti: sending local ICE candidate: %s", candJSON.Candidate)
			candMsg := map[string]interface{}{
				"action": "req",
				"cmd":    "mts",
				"method": "candidate",
				"sid":    sessionID,
				"params": map[string]interface{}{
					"caller": callerID,
					"callee": callee,
					"candidate": map[string]interface{}{
						"candidate":     candJSON.Candidate,
						"sdpMid":        candJSON.SDPMid,
						"sdpMLineIndex": candJSON.SDPMLineIndex,
					},
				},
			}
			_ = wsConn.WriteJSON(candMsg)
		}
	})

	// Add transceivers for video and audio (recvonly)
	if _, err := pc.AddTransceiverFromKind(pion.RTPCodecTypeVideo, pion.RTPTransceiverInit{
		Direction: pion.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := pc.AddTransceiverFromKind(pion.RTPCodecTypeAudio, pion.RTPTransceiverInit{
		Direction: pion.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		_ = conn.Close()
		return nil, err
	}

	offerDesc, err := pc.CreateOffer(nil)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := pc.SetLocalDescription(offerDesc); err != nil {
		_ = conn.Close()
		return nil, err
	}

	// Filter SDP offer: Arenti/Meari devices reject extmap in video description
	filteredSDP := formatOfferSDP(offerDesc.SDP)

	// Send offer MTS message to wake camera and initiate stream
	offerMsg := map[string]interface{}{
		"action": "req",
		"cmd":    "mts",
		"method": "offer",
		"sid":    sessionID,
		"params": map[string]interface{}{
			"caller":     callerID,
			"callee":     callee,
			"devicecode": deviceCode,
			"sdp":        filteredSDP,
			"settings": map[string]interface{}{
				"method": "preview",
				"streams": []map[string]interface{}{
					{"channel": 1, "stream": 0},
				},
			},
		},
	}
	if err := wsConn.WriteJSON(offerMsg); err != nil {
		_ = conn.Close()
		return nil, err
	}

	pc.OnICEConnectionStateChange(func(state pion.ICEConnectionState) {
		Log("arenti: ICE connection state: %s", state)
	})

	pc.OnConnectionStateChange(func(state pion.PeerConnectionState) {
		Log("arenti: peer connection state: %s", state)
		if state == pion.PeerConnectionStateConnected {
			Log("arenti: connected! Sending preview play command...")
			playMsg := map[string]interface{}{
				"action": "req",
				"cmd":    "mts",
				"method": "settings",
				"sid":    sessionID,
				"params": map[string]interface{}{
					"caller": callerID,
					"callee": callee,
					"settings": map[string]interface{}{
						"sid":    1001,
						"method": "preview",
						"streams": []map[string]interface{}{
							{
								"channel": 1,
								"stream":  0,
								"stop":    0,
							},
						},
					},
				},
			}
			_ = wsConn.WriteJSON(playMsg)
		}
	})

	Log("arenti: offer sent (%d bytes), awaiting camera wake up...", len(filteredSDP))

	// 8. Wait for SDP Answer from camera (up to 30s to allow battery cameras to wake up)
	_ = wsConn.SetReadDeadline(time.Now().Add(30 * time.Second))
	var pendingCandidates []pion.ICECandidateInit
	var answerSDP string
	for {
		_, msgBytes, err := wsConn.ReadMessage()
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("arenti: waiting for camera answer timed out or failed: %w", err)
		}

		var rsp struct {
			Action string          `json:"action"`
			Cmd    string          `json:"cmd"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(msgBytes, &rsp); err == nil {
			if rsp.Method == "answer" || rsp.Method == "offer" {
				var p struct {
					SDP string `json:"sdp"`
				}
				if err := json.Unmarshal(rsp.Params, &p); err == nil && p.SDP != "" {
					answerSDP = p.SDP
					Log("arenti: camera answered (%d bytes)", len(answerSDP))
					break
				}
			} else if rsp.Method == "candidate" {
				if cand := parseCandidate(rsp.Params); cand != nil {
					pendingCandidates = append(pendingCandidates, *cand)
				}
			}
		}
	}

	if answerSDP == "" {
		_ = conn.Close()
		return nil, errors.New("arenti: empty SDP answer from camera")
	}

	if err := conn.Conn.SetAnswer(answerSDP); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("arenti: SetAnswer error: %w", err)
	}

	// Apply any candidates received before answer
	for _, cand := range pendingCandidates {
		Log("arenti: applying pending remote ICE candidate: %s", cand.Candidate)
		_ = pc.AddICECandidate(cand)
	}

	// Clear read deadline for continuous streaming
	_ = wsConn.SetReadDeadline(time.Time{})

	// 9. Start 30s hello keepalive ticker
	conn.ticker = time.NewTicker(30 * time.Second)
	go conn.keepaliveLoop()

	// 10. Start WebSocket message reading loop
	go conn.readLoop(pc)

	return conn, nil
}

func (c *Conn) keepaliveLoop() {
	for {
		select {
		case <-c.done:
			return
		case <-c.ticker.C:
			keepalive := map[string]interface{}{
				"action": "req",
				"cmd":    "mts",
				"method": "hello",
				"sid":    c.sessionID,
			}
			_ = c.wsConn.WriteJSON(keepalive)
		}
	}
}

func (c *Conn) readLoop(pc *pion.PeerConnection) {
	for {
		_, msgBytes, err := c.wsConn.ReadMessage()
		if err != nil {
			// WebSocket closed or error -> stop producer
			_ = c.Stop()
			return
		}

		var rsp struct {
			Action string          `json:"action"`
			Cmd    string          `json:"cmd"`
			Method string          `json:"method"`
			ErrID  int             `json:"errid"`
			ErrStr string          `json:"errstr"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(msgBytes, &rsp); err == nil {
			switch rsp.Method {
			case "candidate":
				if cand := parseCandidate(rsp.Params); cand != nil {
					Log("arenti: received remote ICE candidate: %s", cand.Candidate)
					_ = pc.AddICECandidate(*cand)
				}
			case "settings":
				Log("arenti: settings acknowledged by camera")
			case "exception":
				Log("arenti: exception: %d - %s", rsp.ErrID, rsp.ErrStr)
			case "close":
				Log("arenti: camera closed session")
				_ = c.Stop()
				return
			}
		}
	}
}

func parseCandidate(rawParams json.RawMessage) *pion.ICECandidateInit {
	var nested struct {
		Candidate struct {
			Candidate     string  `json:"candidate"`
			SDPMid        *string `json:"sdpMid"`
			SDPMLineIndex *uint16 `json:"sdpMLineIndex"`
		} `json:"candidate"`
	}
	if err := json.Unmarshal(rawParams, &nested); err == nil && nested.Candidate.Candidate != "" {
		return &pion.ICECandidateInit{
			Candidate:     nested.Candidate.Candidate,
			SDPMid:        nested.Candidate.SDPMid,
			SDPMLineIndex: nested.Candidate.SDPMLineIndex,
		}
	}

	var flat struct {
		Candidate     string  `json:"candidate"`
		SDPMid        *string `json:"sdpMid"`
		SDPMLineIndex *uint16 `json:"sdpMLineIndex"`
	}
	if err := json.Unmarshal(rawParams, &flat); err == nil && flat.Candidate != "" {
		return &pion.ICECandidateInit{
			Candidate:     flat.Candidate,
			SDPMid:        flat.SDPMid,
			SDPMLineIndex: flat.SDPMLineIndex,
		}
	}

	return nil
}

// Stop sends MTS "close" to put battery cameras back to sleep (saving battery) and closes resources.
func (c *Conn) Stop() error {
	c.closeOnce.Do(func() {
		close(c.done)
		if c.ticker != nil {
			c.ticker.Stop()
		}

		if c.isBattery {
			// Put battery camera back to sleep
			closeMsg := map[string]interface{}{
				"action": "req",
				"cmd":    "mts",
				"method": "close",
				"sid":    c.sessionID,
				"params": map[string]string{
					"caller": c.callerID,
					"callee": c.callee,
				},
			}
			_ = c.wsConn.WriteJSON(closeMsg)
		}
		_ = c.wsConn.Close()
	})

	return c.Conn.Stop()
}

func (c *Conn) Close() error {
	return c.Stop()
}

// formatOfferSDP removes extmap attributes from video description to ensure camera compatibility.
func formatOfferSDP(sdp string) string {
	lines := strings.Split(sdp, "\r\n")
	var filtered []string
	inVideo := false
	for _, l := range lines {
		if strings.HasPrefix(l, "m=video") {
			inVideo = true
		} else if strings.HasPrefix(l, "m=audio") || strings.HasPrefix(l, "m=application") {
			inVideo = false
		}
		if inVideo && strings.HasPrefix(l, "a=extmap:") {
			continue
		}
		filtered = append(filtered, l)
	}
	return strings.Join(filtered, "\r\n")
}
