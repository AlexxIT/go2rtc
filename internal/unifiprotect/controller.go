package unifiprotect

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	websocketPath     = "/camera/1.0/ws"
	websocketProtocol = "secure_transfer"
	maxControlMessage = 8 * 1024 * 1024
)

type controlMessage struct {
	From             string          `json:"from"`
	FunctionName     string          `json:"functionName"`
	InResponseTo     int64           `json:"inResponseTo"`
	MessageID        int64           `json:"messageId"`
	Payload          json.RawMessage `json:"payload"`
	ResponseExpected bool            `json:"responseExpected"`
	To               string          `json:"to"`
}

type outgoingMessage struct {
	From             string `json:"from"`
	FunctionName     string `json:"functionName"`
	InResponseTo     int64  `json:"inResponseTo"`
	MessageID        int64  `json:"messageId"`
	Payload          any    `json:"payload"`
	ResponseExpected bool   `json:"responseExpected"`
	TimeStamp        string `json:"timeStamp"`
	To               string `json:"to"`
}

type cameraFeatures struct {
	AudioCodecs    []string `json:"audioCodecs"`
	OpusSampleRate []int    `json:"opusSampleRates"`
}

type helloPayload struct {
	Features        cameraFeatures `json:"features"`
	ProtocolVersion int            `json:"protocolVersion"`
}

type controller struct {
	manager  *Manager
	upgrader websocket.Upgrader
}

func newController(manager *Manager) *controller {
	return &controller{
		manager: manager,
		upgrader: websocket.Upgrader{
			Subprotocols: []string{websocketProtocol},
			CheckOrigin:  func(*http.Request) bool { return true },
		},
	}
}

func (c *controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != websocketPath ||
		!websocket.IsWebSocketUpgrade(r) ||
		!slices.Contains(websocket.Subprotocols(r), websocketProtocol) {
		http.Error(w, "bad camera websocket request", http.StatusBadRequest)
		return
	}

	mac, err := normalizeMAC(r.Header.Get("camera-mac"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(maxControlMessage)

	s := &session{
		manager:        c.manager,
		conn:           conn,
		mac:            mac,
		cameraIP:       remoteHost(r.RemoteAddr),
		controllerHost: requestHost(r.Host),
		done:           make(chan struct{}),
		nextMessageID:  time.Now().UnixMilli(),
	}
	if s.controllerHost == "" {
		s.controllerHost = requestHost(conn.LocalAddr().String())
	}

	go s.run()
}

type session struct {
	manager *Manager
	conn    *websocket.Conn

	mac            string
	cameraIP       string
	controllerHost string
	opusRate       int

	writeMu       sync.Mutex
	nextMessageID int64
	paramID       int64
	closeOnce     sync.Once
	done          chan struct{}
}

func (s *session) run() {
	defer func() {
		s.close()
		s.manager.removeSession(s)
	}()

	for {
		var msg controlMessage
		if err := s.conn.ReadJSON(&msg); err != nil {
			return
		}
		if err := s.handle(msg); err != nil {
			log.Debug().Err(err).Str("camera", s.mac).Msg("[unifi-protect] control message")
			return
		}
	}
}

func (s *session) handle(msg controlMessage) error {
	switch msg.FunctionName {
	case "ubnt_avclient_timeSync":
		now := time.Now().UnixMilli()
		return s.respond(msg, map[string]any{"t1": now, "t2": now})

	case "ubnt_avclient_hello":
		var hello helloPayload
		if err := json.Unmarshal(msg.Payload, &hello); err != nil {
			return err
		}
		s.opusRate = preferredOpusRate(hello.Features)

		if err := s.respond(msg, map[string]any{
			"protocolVersion":   hello.ProtocolVersion,
			"controllerName":    "go2rtc",
			"controllerUuid":    s.manager.controllerUUID,
			"controllerVersion": s.manager.controllerVersion,
			"overrideUuid":      true,
		}); err != nil {
			return err
		}

		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-timer.C:
		case <-s.done:
			timer.Stop()
			return net.ErrClosed
		}

		id, err := s.send("ubnt_avclient_paramAgreement", map[string]any{
			"enableStatusCodes":   true,
			"useHeartbeats":       false,
			"heartbeatsTimeoutMs": 60000,
		}, true, 0, "ubnt_avclient")
		s.paramID = id
		return err

	case "ubnt_avclient_paramAgreement":
		if msg.InResponseTo == s.paramID && s.paramID != 0 {
			s.manager.addSession(s)
			return nil
		}

	case "ChangeVideoSettings":
		if msg.InResponseTo != 0 {
			log.Debug().Str("camera", s.mac).Msg("[unifi-protect] video settings applied")
		}
	}

	if msg.ResponseExpected && msg.InResponseTo == 0 {
		var payload any = map[string]any{}
		if len(msg.Payload) != 0 {
			_ = json.Unmarshal(msg.Payload, &payload)
		}
		return s.respond(msg, payload)
	}
	return nil
}

func (s *session) respond(msg controlMessage, payload any) error {
	to := msg.From
	if to == "" {
		to = "ubnt_avclient"
	}
	_, err := s.send(msg.FunctionName, payload, false, msg.MessageID, to)
	return err
}

func (s *session) send(function string, payload any, responseExpected bool, inResponseTo int64, to string) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	select {
	case <-s.done:
		return 0, net.ErrClosed
	default:
	}

	s.nextMessageID++
	msg := outgoingMessage{
		From:             "UniFiVideo",
		FunctionName:     function,
		InResponseTo:     inResponseTo,
		MessageID:        s.nextMessageID,
		Payload:          payload,
		ResponseExpected: responseExpected,
		TimeStamp:        time.Now().UTC().Format("2006-01-02T15:04:05.000-07:00"),
		To:               to,
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return 0, err
	}
	log.Debug().Str("camera", s.mac).Str("function", function).Int64("message_id", msg.MessageID).Msg("[unifi-protect] control send")
	return msg.MessageID, s.conn.WriteMessage(websocket.BinaryMessage, b)
}

func (s *session) startStream(channel, destination, token string, audio bool) error {
	parameters := map[string]any{
		"streamName":    token,
		"suppressAudio": !audio,
		"suppressVideo": false,
		"withOpus":      audio && s.opusRate != 0,
	}
	if audio && s.opusRate != 0 {
		parameters["opusSampleRate"] = s.opusRate
	}

	_, err := s.send("ChangeVideoSettings", map[string]any{
		"video": map[string]any{
			channel: map[string]any{
				"avSerializer": map[string]any{
					"type":         "extendedFlv",
					"parameters":   parameters,
					"destinations": []string{destination},
				},
				"type": "h264",
			},
		},
	}, true, 0, "ubnt_avclient")
	return err
}

func (s *session) stopStreams(channels []string) error {
	video := make(map[string]any, len(channels))
	for _, channel := range channels {
		parameters := map[string]any{"withOpus": s.opusRate != 0}
		if s.opusRate != 0 {
			parameters["opusSampleRate"] = s.opusRate
		}
		video[channel] = map[string]any{
			"avSerializer": map[string]any{
				"type":         "extendedFlv",
				"parameters":   parameters,
				"destinations": []string{"file:///dev/null"},
			},
		}
	}
	if len(video) == 0 {
		return nil
	}
	_, err := s.send("ChangeVideoSettings", map[string]any{"video": video}, true, 0, "ubnt_avclient")
	return err
}

func (s *session) close() {
	s.closeOnce.Do(func() {
		close(s.done)
		_ = s.conn.Close()
	})
}

func preferredOpusRate(features cameraFeatures) int {
	if !slices.Contains(features.AudioCodecs, "opus") {
		return 0
	}
	rate := 0
	for _, v := range features.OpusSampleRate {
		if v > rate {
			rate = v
		}
	}
	return rate
}

func remoteHost(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return normalizeIP(host)
	}
	return normalizeIP(address)
}

func requestHost(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return address
}

func normalizeIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	if v4 := parsed.To4(); v4 != nil {
		return v4.String()
	}
	return parsed.String()
}

var errSessionUnavailable = errors.New("unifi-protect: camera session unavailable")
