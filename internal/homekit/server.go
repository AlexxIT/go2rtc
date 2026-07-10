package homekit

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg"
	srtp2 "github.com/AlexxIT/go2rtc/internal/srtp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/hap/hds"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/AlexxIT/go2rtc/pkg/homekit"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
	pion "github.com/pion/webrtc/v4"
)

type server struct {
	hap  *hap.Server // server for HAP connection and encryption
	mdns *mdns.ServiceEntry

	pairings []string // pairings list
	conns    []any
	mu       sync.Mutex

	accessory *hap.Accessory // HAP accessory
	consumer  *homekit.Consumer
	proxyURL  string
	setupID   string
	stream    string // stream name from YAML

	// Experimental HKSV open-source (WebRTC + HEVC + CMAF)
	hksv      bool
	webrtc    *homekit.WebRTCManager
	recording *homekit.RecordingManager
	// last write-response values keyed by characteristic IID
	wrValues map[uint64]any
}

func (s *server) MarshalJSON() ([]byte, error) {
	v := struct {
		Name       string `json:"name"`
		DeviceID   string `json:"device_id"`
		Paired     int    `json:"paired,omitempty"`
		CategoryID string `json:"category_id,omitempty"`
		SetupCode  string `json:"setup_code,omitempty"`
		SetupID    string `json:"setup_id,omitempty"`
		Conns      []any  `json:"connections,omitempty"`
	}{
		Name:       s.mdns.Name,
		DeviceID:   s.mdns.Info[hap.TXTDeviceID],
		CategoryID: s.mdns.Info[hap.TXTCategory],
		Paired:     len(s.pairings),
		Conns:      s.conns,
	}
	if v.Paired == 0 {
		v.SetupCode = s.hap.Pin
		v.SetupID = s.setupID
	}
	return json.Marshal(v)
}

func (s *server) Handle(w http.ResponseWriter, r *http.Request) {
	conn, rw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}

	defer conn.Close()

	// Fix reading from Body after Hijack.
	r.Body = io.NopCloser(rw)

	switch r.RequestURI {
	case hap.PathPairSetup:
		id, key, err := s.hap.PairSetup(r, rw)
		if err != nil {
			log.Error().Err(err).Caller().Send()
			return
		}

		s.AddPair(id, key, hap.PermissionAdmin)

	case hap.PathPairVerify:
		id, key, err := s.hap.PairVerify(r, rw)
		if err != nil {
			log.Debug().Err(err).Caller().Send()
			return
		}

		log.Debug().Str("stream", s.stream).Str("client_id", id).Msgf("[homekit] %s: new conn", conn.RemoteAddr())

		controller, err := hap.NewConn(conn, rw, key, false)
		if err != nil {
			log.Error().Err(err).Caller().Send()
			return
		}

		s.AddConn(controller)
		defer s.DelConn(controller)

		var handler homekit.HandlerFunc

		switch {
		case s.accessory != nil:
			handler = homekit.ServerHandler(s)
		case s.proxyURL != "":
			client, err := hap.Dial(s.proxyURL)
			if err != nil {
				log.Error().Err(err).Caller().Send()
				return
			}
			handler = homekit.ProxyHandler(s, client.Conn)
		}

		// If your iPhone goes to sleep, it will be an EOF error.
		if err = handler(controller); err != nil && !errors.Is(err, io.EOF) {
			log.Error().Err(err).Caller().Send()
			return
		}
	}
}

type logger struct {
	v any
}

func (l logger) String() string {
	switch v := l.v.(type) {
	case *hap.Conn:
		return "hap " + v.RemoteAddr().String()
	case *hds.Conn:
		return "hds " + v.RemoteAddr().String()
	case *homekit.Consumer:
		return "rtp " + v.RemoteAddr
	}
	return "unknown"
}

func (s *server) AddConn(v any) {
	log.Trace().Str("stream", s.stream).Msgf("[homekit] add conn %s", logger{v})
	s.mu.Lock()
	s.conns = append(s.conns, v)
	s.mu.Unlock()
}

func (s *server) DelConn(v any) {
	log.Trace().Str("stream", s.stream).Msgf("[homekit] del conn %s", logger{v})
	s.mu.Lock()
	if i := slices.Index(s.conns, v); i >= 0 {
		s.conns = slices.Delete(s.conns, i, i+1)
	}
	s.mu.Unlock()
}

func (s *server) UpdateStatus() {
	// true status is important, or device may be offline in Apple Home
	if len(s.pairings) == 0 {
		s.mdns.Info[hap.TXTStatusFlags] = hap.StatusNotPaired
	} else {
		s.mdns.Info[hap.TXTStatusFlags] = hap.StatusPaired
	}
}

func (s *server) pairIndex(id string) int {
	id = "client_id=" + id
	for i, pairing := range s.pairings {
		if strings.HasPrefix(pairing, id) {
			return i
		}
	}
	return -1
}

func (s *server) GetPair(id string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	if i := s.pairIndex(id); i >= 0 {
		query, _ := url.ParseQuery(s.pairings[i])
		b, _ := hex.DecodeString(query.Get("client_public"))
		return b
	}
	return nil
}

func (s *server) AddPair(id string, public []byte, permissions byte) {
	log.Debug().Str("stream", s.stream).Msgf("[homekit] add pair id=%s public=%x perm=%d", id, public, permissions)

	s.mu.Lock()
	if s.pairIndex(id) < 0 {
		s.pairings = append(s.pairings, fmt.Sprintf(
			"client_id=%s&client_public=%x&permissions=%d", id, public, permissions,
		))
		s.UpdateStatus()
		s.PatchConfig()
	}
	s.mu.Unlock()
}

func (s *server) DelPair(id string) {
	log.Debug().Str("stream", s.stream).Msgf("[homekit] del pair id=%s", id)

	s.mu.Lock()
	if i := s.pairIndex(id); i >= 0 {
		s.pairings = append(s.pairings[:i], s.pairings[i+1:]...)
		s.UpdateStatus()
		s.PatchConfig()
	}
	s.mu.Unlock()
}

func (s *server) PatchConfig() {
	if err := app.PatchConfig([]string{"homekit", s.stream, "pairings"}, s.pairings); err != nil {
		log.Error().Err(err).Msgf(
			"[homekit] can't save %s pairings=%v", s.stream, s.pairings,
		)
	}
}

func (s *server) GetAccessories(_ net.Conn) []*hap.Accessory {
	return []*hap.Accessory{s.accessory}
}

func (s *server) GetCharacteristic(conn net.Conn, aid uint8, iid uint64) any {
	log.Trace().Str("stream", s.stream).Msgf("[homekit] get char aid=%d iid=0x%x", aid, iid)

	char := s.accessory.GetCharacterByID(iid)
	if char == nil {
		log.Warn().Msgf("[homekit] get unknown characteristic: %d", iid)
		return nil
	}

	// Prefer last write-response payload when present
	if s.wrValues != nil {
		if v, ok := s.wrValues[iid]; ok {
			return v
		}
	}

	switch char.Type {
	case camera.TypeSetupEndpoints:
		consumer := s.consumer
		if consumer == nil {
			return nil
		}

		answer := consumer.GetAnswer()
		v, err := tlv8.MarshalBase64(answer)
		if err != nil {
			return nil
		}

		return v

	case camera.TypeWebRTCNumberOfActiveSessions:
		if s.webrtc != nil {
			return s.webrtc.ActiveCount()
		}
		return 0

	case camera.TypeCameraClientCertificateStatus:
		if s.recording != nil {
			v, err := tlv8.MarshalBase64(camera.CameraClientCertificateStatusValue{
				NeedsUpdate: s.recording.Creds.NeedsUpdate(),
			})
			if err == nil {
				return v
			}
		}

	case camera.TypeBufferEventSequenceNumber:
		if s.recording != nil {
			return s.recording.EventSequence()
		}
		return uint32(0)
	}

	return char.Value
}

func (s *server) SetCharacteristic(conn net.Conn, aid uint8, iid uint64, value any) {
	log.Trace().Str("stream", s.stream).Msgf("[homekit] set char aid=%d iid=0x%x value=%v", aid, iid, value)

	char := s.accessory.GetCharacterByID(iid)
	if char == nil {
		log.Warn().Msgf("[homekit] set unknown characteristic: %d", iid)
		return
	}

	switch char.Type {
	case camera.TypeSetupEndpoints:
		var offer camera.SetupEndpointsRequest
		if err := tlv8.UnmarshalBase64(value, &offer); err != nil {
			return
		}

		consumer := homekit.NewConsumer(conn, srtp2.Server)
		consumer.SetOffer(&offer)
		s.consumer = consumer

	case camera.TypeSelectedStreamConfiguration:
		var conf camera.SelectedStreamConfiguration
		if err := tlv8.UnmarshalBase64(value, &conf); err != nil {
			return
		}

		log.Trace().Str("stream", s.stream).Msgf("[homekit] stream id=%x cmd=%d", conf.Control.SessionID, conf.Control.Command)

		switch conf.Control.Command {
		case camera.SessionCommandEnd:
			for _, consumer := range s.conns {
				if consumer, ok := consumer.(*homekit.Consumer); ok {
					if consumer.SessionID() == conf.Control.SessionID {
						_ = consumer.Stop()
						return
					}
				}
			}

		case camera.SessionCommandStart:
			consumer := s.consumer
			if consumer == nil {
				return
			}

			if !consumer.SetConfig(&conf) {
				log.Warn().Msgf("[homekit] wrong config")
				return
			}

			s.AddConn(consumer)

			stream := streams.Get(s.stream)
			if err := stream.AddConsumer(consumer); err != nil {
				return
			}

			go func() {
				_, _ = consumer.WriteTo(nil)
				stream.RemoveConsumer(consumer)

				s.DelConn(consumer)
			}()
		}

	case camera.TypeWebRTCSolicitOffer:
		s.handleWebRTCSolicitOffer(iid, value)

	case camera.TypeWebRTCProvideAnswer:
		s.handleWebRTCProvideAnswer(iid, value)

	case camera.TypeWebRTCStreamingControl:
		s.handleWebRTCStreamingControl(iid, value)

	case camera.TypeWebRTCReoffer:
		s.handleWebRTCReoffer(iid, value)

	case camera.TypeWebRTCUpdateSession:
		s.handleWebRTCUpdateSession(iid, value)

	case camera.TypeRTPStreamingControl:
		s.handleRTPStreamingControl(iid, value)

	case camera.TypeCameraClientCSR:
		s.handleCameraClientCSR(iid, value)

	case camera.TypeCameraClientCertificate:
		s.handleCameraClientCertificate(value)

	case camera.TypeCameraKey:
		s.handleCameraKey(value)

	case camera.TypeCameraRecordingPublishingPoint:
		s.handlePublishingPoint(value)

	case camera.TypeStreamingEnabled, camera.TypeHomeKitCameraActive,
		camera.TypeMotionEnabled, camera.TypeCameraOperatingModeIndicator:
		_ = char.Write(value)
		_ = char.NotifyListeners(conn)

	case camera.TypeActive:
		active := truthy(value)
		if s.recording != nil {
			s.recording.SetRecordingActive(active)
		}
		if active {
			char.Value = uint8(1)
		} else {
			char.Value = uint8(0)
		}
		_ = char.NotifyListeners(conn)

	case camera.TypeRecordingAudioActive:
		active := truthy(value)
		if s.recording != nil {
			s.recording.SetAudioActive(active)
		}
		if active {
			char.Value = uint8(1)
		} else {
			char.Value = uint8(0)
		}
		_ = char.NotifyListeners(conn)

	case camera.TypeCameraZones:
		_ = char.Write(value)

	case camera.TypeBufferActivityCommand:
		s.handleBufferActivity(value)

	case camera.TypeBufferUploadCommand:
		s.handleBufferUpload(iid, value)

	case camera.TypeBufferEventCommand:
		s.handleBufferEvent(iid, value)
	}
}

func (s *server) setWriteResponse(iid uint64, v any) {
	if s.wrValues == nil {
		s.wrValues = map[uint64]any{}
	}
	encoded, err := tlv8.MarshalBase64(v)
	if err != nil {
		return
	}
	s.wrValues[iid] = encoded
	if char := s.accessory.GetCharacterByID(iid); char != nil {
		char.Value = encoded
	}
}

func (s *server) handleWebRTCSolicitOffer(iid uint64, value any) {
	if s.webrtc == nil {
		s.setWriteResponse(iid, camera.WebRTCSolicitOfferResponse{Status: camera.WebRTCSolicitError})
		return
	}

	var req camera.WebRTCSolicitOfferRequest
	if err := tlv8.UnmarshalBase64(value, &req); err != nil {
		s.setWriteResponse(iid, camera.WebRTCSolicitOfferResponse{Status: camera.WebRTCSolicitError})
		return
	}

	// Check global streaming gates
	if !s.streamingAllowed() {
		s.setWriteResponse(iid, camera.WebRTCSolicitOfferResponse{Status: camera.WebRTCSolicitPrivacyModeActive})
		return
	}

	res, err := s.webrtc.SolicitOffer(req.Options.SFrameEnabled)
	if err != nil || res == nil {
		s.setWriteResponse(iid, camera.WebRTCSolicitOfferResponse{Status: camera.WebRTCSolicitError})
		return
	}

	s.setWriteResponse(iid, res)
	s.updateWebRTCSessionCount()
	log.Debug().Str("stream", s.stream).Msgf("[homekit] webrtc solicit-offer status=%d sessions=%d", res.Status, s.webrtc.ActiveCount())
}

func (s *server) handleWebRTCProvideAnswer(iid uint64, value any) {
	if s.webrtc == nil {
		s.setWriteResponse(iid, camera.WebRTCProvideAnswerResponse{Status: camera.WebRTCStatusError})
		return
	}

	var req camera.WebRTCProvideAnswerRequest
	if err := tlv8.UnmarshalBase64(value, &req); err != nil {
		s.setWriteResponse(iid, camera.WebRTCProvideAnswerResponse{Status: camera.WebRTCStatusError})
		return
	}

	res := s.webrtc.ProvideAnswer(&req)
	s.setWriteResponse(iid, res)

	if res.Status != camera.WebRTCStatusSuccess {
		return
	}

	sess := s.webrtc.GetSession(req.SessionIdentifier)
	if sess == nil || sess.Conn == nil {
		return
	}

	stream := streams.Get(s.stream)
	if stream == nil {
		return
	}

	s.AddConn(sess.Conn)
	if err := stream.AddConsumer(sess.Conn); err != nil {
		log.Warn().Err(err).Str("stream", s.stream).Msg("[homekit] webrtc add consumer")
		return
	}

	sessionID := req.SessionIdentifier
	conn := sess.Conn
	conn.Listen(func(msg any) {
		state, ok := msg.(pion.PeerConnectionState)
		if !ok {
			return
		}
		switch state {
		case pion.PeerConnectionStateDisconnected, pion.PeerConnectionStateFailed, pion.PeerConnectionStateClosed:
			stream.RemoveConsumer(conn)
			s.DelConn(conn)
			_ = s.webrtc.EndSession(sessionID)
			s.updateWebRTCSessionCount()
		}
	})

	log.Debug().Str("stream", s.stream).Msg("[homekit] webrtc provide-answer ok")
}

func (s *server) handleWebRTCStreamingControl(iid uint64, value any) {
	if s.webrtc == nil {
		s.setWriteResponse(iid, camera.WebRTCStreamingControlResponse{Status: camera.WebRTCStatusError})
		return
	}

	var req camera.WebRTCStreamingControlRequest
	if err := tlv8.UnmarshalBase64(value, &req); err != nil {
		s.setWriteResponse(iid, camera.WebRTCStreamingControlResponse{Status: camera.WebRTCStatusError})
		return
	}

	if req.Command == camera.WebRTCCommandEnd {
		res := s.webrtc.EndSession(req.SessionIdentifier)
		s.setWriteResponse(iid, res)
		s.updateWebRTCSessionCount()
		return
	}

	s.setWriteResponse(iid, camera.WebRTCStreamingControlResponse{
		SessionIdentifier: req.SessionIdentifier,
		Status:            camera.WebRTCStatusError,
	})
}

func (s *server) handleWebRTCReoffer(iid uint64, value any) {
	if s.webrtc == nil {
		s.setWriteResponse(iid, camera.WebRTCReofferResponse{Status: camera.WebRTCStatusError})
		return
	}
	var req camera.WebRTCReofferRequest
	if err := tlv8.UnmarshalBase64(value, &req); err != nil {
		s.setWriteResponse(iid, camera.WebRTCReofferResponse{Status: camera.WebRTCStatusError})
		return
	}
	s.setWriteResponse(iid, s.webrtc.Reoffer(&req))
}

func (s *server) handleWebRTCUpdateSession(iid uint64, value any) {
	if s.webrtc == nil {
		s.setWriteResponse(iid, camera.WebRTCUpdateSessionResponse{Status: camera.WebRTCStatusError})
		return
	}
	var req camera.WebRTCUpdateSessionRequest
	if err := tlv8.UnmarshalBase64(value, &req); err != nil {
		s.setWriteResponse(iid, camera.WebRTCUpdateSessionResponse{Status: camera.WebRTCStatusError})
		return
	}
	s.setWriteResponse(iid, s.webrtc.UpdateSession(&req))
}

func (s *server) handleRTPStreamingControl(iid uint64, value any) {
	var req camera.RTPStreamingControlRequest
	if err := tlv8.UnmarshalBase64(value, &req); err != nil {
		s.setWriteResponse(iid, camera.RTPStreamingControlResponse{Status: camera.StreamStatusError})
		return
	}

	// Multi-tier RTP control: map Start onto the classic consumer path when possible
	status := byte(camera.StreamStatusSuccess)
	switch req.Command {
	case camera.RTPStreamCommandEnd:
		for _, consumer := range s.conns {
			if consumer, ok := consumer.(*homekit.Consumer); ok {
				if consumer.SessionID() == req.SessionIdentifier {
					_ = consumer.Stop()
					break
				}
			}
		}
	case camera.RTPStreamCommandStart:
		if !s.streamingAllowed() {
			status = camera.StreamStatusError
		}
		// Full multi-tier encoder reconfiguration is left to the stream source;
		// Start still relies on Setup Endpoints + classic selected stream for media
	default:
		status = camera.StreamStatusError
	}

	s.setWriteResponse(iid, camera.RTPStreamingControlResponse{
		SessionIdentifier: req.SessionIdentifier,
		Status:            status,
	})
}

func (s *server) handleCameraClientCSR(iid uint64, value any) {
	if s.recording == nil {
		return
	}
	var req camera.CameraClientCSRRequest
	if err := tlv8.UnmarshalBase64(value, &req); err != nil {
		return
	}
	csrDER, sig, err := s.recording.Creds.HandleCSR([]byte(req.Nonce))
	if err != nil {
		log.Warn().Err(err).Msg("[homekit] csr")
		return
	}
	s.setWriteResponse(iid, camera.CameraClientCSRResponse{
		CSR:            string(csrDER),
		NonceSignature: string(sig),
	})
}

func (s *server) handleCameraClientCertificate(value any) {
	if s.recording == nil {
		return
	}
	var req camera.CameraClientCertificateRequest
	if err := tlv8.UnmarshalBase64(value, &req); err != nil {
		return
	}
	s.recording.Creds.InstallClientCertificate([]byte(req.ClientCertificate), []byte(req.CA))
	if char := s.accessory.GetCharacter(camera.TypeCameraClientCertificateStatus); char != nil {
		_ = char.Set(camera.CameraClientCertificateStatusValue{NeedsUpdate: false})
	}
}

func (s *server) handleCameraKey(value any) {
	if s.recording == nil {
		return
	}
	var req camera.CameraKeyValue
	if err := tlv8.UnmarshalBase64(value, &req); err != nil {
		return
	}
	id := s.recording.Creds.SetKey([]byte(req.Key), req.KeyNumber)
	if char := s.accessory.GetCharacter(camera.TypeCameraKeyID); char != nil {
		_ = char.Set(camera.CameraKeyIDValue{KeyID: id})
	}
}

func (s *server) handlePublishingPoint(value any) {
	if s.recording == nil {
		return
	}
	var req camera.CameraRecordingPublishingPointValue
	if err := tlv8.UnmarshalBase64(value, &req); err != nil {
		// store raw value if not TLV8-shaped
		if char := s.accessory.GetCharacter(camera.TypeCameraRecordingPublishingPoint); char != nil {
			char.Value = value
		}
		return
	}
	var cas [][]byte
	for _, c := range req.ServerCACertificates {
		cas = append(cas, []byte(c.Certificate))
	}
	s.recording.Creds.SetPublishingPoint(req.URL, cas)
	if char := s.accessory.GetCharacter(camera.TypeCameraRecordingPublishingPoint); char != nil {
		char.Value = value
	}
	log.Debug().Str("stream", s.stream).Str("url", req.URL).Msg("[homekit] cmaf publishing point set")
}

func (s *server) handleBufferActivity(value any) {
	if s.recording == nil {
		return
	}
	var req camera.BufferActivityCommandRequest
	if err := tlv8.UnmarshalBase64(value, &req); err != nil {
		return
	}
	s.recording.HandleActivity(&req)
}

func (s *server) handleBufferUpload(iid uint64, value any) {
	if s.recording == nil {
		s.setWriteResponse(iid, camera.BufferUploadCommandResponse{})
		return
	}
	var req camera.BufferUploadCommandRequest
	if err := tlv8.UnmarshalBase64(value, &req); err != nil {
		s.setWriteResponse(iid, camera.BufferUploadCommandResponse{})
		return
	}
	res := s.recording.HandleUpload(&req)
	s.setWriteResponse(iid, res)
	log.Debug().Str("stream", s.stream).
		Uint64("session", req.SessionID).
		Uint64("clip", res.ClipID).
		Uint8("cmd", req.Command).
		Msg("[homekit] buffer upload command")
}

func (s *server) handleBufferEvent(iid uint64, value any) {
	if s.recording == nil {
		s.setWriteResponse(iid, camera.BufferEventCommandResponse{})
		return
	}
	var req camera.BufferEventCommandRequest
	if err := tlv8.UnmarshalBase64(value, &req); err != nil {
		s.setWriteResponse(iid, camera.BufferEventCommandResponse{})
		return
	}
	s.setWriteResponse(iid, s.recording.HandleEventCommand(&req))
}

func truthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case int:
		return v != 0
	case uint8:
		return v != 0
	case uint64:
		return v != 0
	case string:
		return v == "1" || v == "true"
	default:
		return false
	}
}

// startRecordingBuffer attaches a ring-buffer consumer to the source stream
// Retries until the stream can provide matching tracks
func (s *server) startRecordingBuffer() {
	if s.recording == nil {
		return
	}
	cons := s.recording.EnsureConsumer()
	for {
		stream := streams.Get(s.stream)
		if stream == nil {
			return
		}
		if err := stream.AddConsumer(cons); err != nil {
			log.Debug().Err(err).Str("stream", s.stream).Msg("[homekit] recording buffer wait for tracks")
			time.Sleep(2 * time.Second)
			continue
		}
		log.Info().Str("stream", s.stream).Msg("[homekit] recording pre-buffer started")
		return
	}
}

// notifyEventSequence updates the HAP event sequence characteristic
func (s *server) notifyEventSequence(seq uint32) {
	if s.accessory == nil {
		return
	}
	if char := s.accessory.GetCharacter(camera.TypeBufferEventSequenceNumber); char != nil {
		char.Value = seq
		_ = char.NotifyListeners(nil)
	}
}

func (s *server) streamingAllowed() bool {
	if s.accessory == nil {
		return true
	}
	// HomeKit Camera Active
	if char := s.accessory.GetCharacter(camera.TypeHomeKitCameraActive); char != nil {
		if v, err := char.ReadBool(); err == nil && !v {
			return false
		}
	}
	// Global / service Streaming Enabled (any false blocks)
	for _, srv := range s.accessory.Services {
		for _, char := range srv.Characters {
			if char.Type == camera.TypeStreamingEnabled {
				if v, err := char.ReadBool(); err == nil && !v {
					return false
				}
			}
		}
	}
	return true
}

func (s *server) updateWebRTCSessionCount() {
	if s.accessory == nil || s.webrtc == nil {
		return
	}
	if char := s.accessory.GetCharacter(camera.TypeWebRTCNumberOfActiveSessions); char != nil {
		char.Value = s.webrtc.ActiveCount()
		_ = char.NotifyListeners(nil)
	}
}

func (s *server) GetImage(conn net.Conn, width, height int) []byte {
	log.Trace().Str("stream", s.stream).Msgf("[homekit] get image width=%d height=%d", width, height)

	stream := streams.Get(s.stream)
	cons := magic.NewKeyframe()

	if err := stream.AddConsumer(cons); err != nil {
		return nil
	}

	once := &core.OnceBuffer{} // init and first frame
	_, _ = cons.WriteTo(once)
	b := once.Buffer()

	stream.RemoveConsumer(cons)

	switch cons.CodecName() {
	case core.CodecH264, core.CodecH265:
		var err error
		if b, err = ffmpeg.JPEGWithScale(b, width, height); err != nil {
			return nil
		}
	}

	return b
}

func calcName(name, seed string) string {
	if name != "" {
		return name
	}
	b := sha512.Sum512([]byte(seed))
	return fmt.Sprintf("go2rtc-%02X%02X", b[0], b[2])
}

func calcDeviceID(deviceID, seed string) string {
	if deviceID != "" {
		if len(deviceID) >= 17 {
			// 1. Returd device_id as is (ex. AA:BB:CC:DD:EE:FF)
			return deviceID
		}
		// 2. Use device_id as seed if not zero
		seed = deviceID
	}
	b := sha512.Sum512([]byte(seed))
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", b[32], b[34], b[36], b[38], b[40], b[42])
}

func calcDevicePrivate(private, seed string) []byte {
	if private != "" {
		// 1. Decode private from HEX string
		if b, _ := hex.DecodeString(private); len(b) == ed25519.PrivateKeySize {
			// 2. Return if OK
			return b
		}
		// 3. Use private as seed if not zero
		seed = private
	}
	b := sha512.Sum512([]byte(seed))
	return ed25519.NewKeyFromSeed(b[:ed25519.SeedSize])
}

func calcSetupID(seed string) string {
	b := sha512.Sum512([]byte(seed))
	return fmt.Sprintf("%02X%02X", b[44], b[46])
}

func calcCategoryID(categoryID string) string {
	switch categoryID {
	case "bridge":
		return hap.CategoryBridge
	case "doorbell":
		return hap.CategoryDoorbell
	}
	if core.Atoi(categoryID) > 0 {
		return categoryID
	}
	return hap.CategoryCamera
}
