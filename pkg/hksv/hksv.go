// Package hksv provides a reusable HomeKit Secure Video server library.
//
// It implements HKSV recording (fMP4 over HDS DataStream), motion detection,
// and integrates with the HAP protocol for HomeKit pairing and communication.
//
// Usage:
//
//	srv, err := hksv.NewServer(hksv.Config{
//	    StreamName: "camera1",
//	    Pin:        "27041991",
//	    HKSV:       true,
//	    MotionMode: "detect",
//	    Streams:    myStreamProvider,
//	    Logger:     logger,
//	    Port:       8080,
//	})
//	// Register srv.Handle as HTTP handler for HAP paths
//	// Advertise srv.MDNSEntry() via mDNS
//
// Author: Sergei "svk" Krashevich <svk@svk.su>
package hksv

import (
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

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/hap/hds"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/AlexxIT/go2rtc/pkg/homekit"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
	"github.com/rs/zerolog"
)

// StreamProvider provides access to media streams.
// The host application implements this to connect the HKSV library
// to its own stream management system.
type StreamProvider interface {
	// AddConsumer connects a consumer to the named stream.
	AddConsumer(streamName string, consumer core.Consumer) error
	// RemoveConsumer disconnects a consumer from the named stream.
	RemoveConsumer(streamName string, consumer core.Consumer)
}

// PairingStore persists HAP pairing data.
type PairingStore interface {
	SavePairings(streamName string, pairings []string) error
}

// SnapshotProvider generates JPEG snapshots for HomeKit /resource requests.
type SnapshotProvider interface {
	GetSnapshot(streamName string, width, height int) ([]byte, error)
}

// LiveStreamHandler handles live-streaming requests (SetupEndpoints, SelectedStreamConfiguration).
// Implementation is external because it depends on SRTP.
type LiveStreamHandler interface {
	// SetupEndpoints handles a SetupEndpoints request (ch118).
	// Returns the response to store as characteristic value.
	SetupEndpoints(conn net.Conn, offer *camera.SetupEndpointsRequest) (any, error)

	// GetEndpointsResponse returns the current endpoints response (for GET requests).
	GetEndpointsResponse() any

	// StartStream starts RTP streaming with the given configuration (ch117 command=start).
	// The connTracker is used to register/unregister the live stream connection.
	StartStream(streamName string, conf *camera.SelectedStreamConfiguration, connTracker ConnTracker) error

	// StopStream stops a stream matching the given session ID.
	StopStream(sessionID string, connTracker ConnTracker) error
}

// ConnTracker allows the live stream handler to track connections on the server.
type ConnTracker interface {
	AddConn(v any)
	DelConn(v any)
}

// Config for creating an HKSV server.
type Config struct {
	StreamName      string
	Pin             string   // HomeKit pairing PIN (e.g., "27041991")
	Name            string   // mDNS display name (auto-generated if empty)
	DeviceID        string   // MAC-like device ID (auto-generated if empty)
	DevicePrivate   string   // ed25519 private key hex (auto-generated if empty)
	CategoryID      string   // "camera" or "doorbell"
	Pairings        []string // pre-existing pairings
	ProxyURL        string   // if set, acts as transparent proxy (no local accessory)
	HKSV            bool
	MotionMode      string  // "api", "continuous", "detect"
	MotionThreshold float64 // ratio threshold for "detect" mode (default 2.0)
	Speaker         *bool   // include Speaker service for 2-way audio (default false)
	UserAgent       string  // for mDNS TXTModel field
	Version         string  // for accessory firmware version

	// Dependencies (injected by host)
	Streams    StreamProvider
	Store      PairingStore      // optional, nil = no persistence
	Snapshots  SnapshotProvider  // optional, nil = no snapshots
	LiveStream LiveStreamHandler // optional, nil = HKSV only (no live streaming)
	Logger     zerolog.Logger

	// Network
	Port uint16 // HAP HTTP port
}

// Server is a complete HKSV camera server.
type Server struct {
	hap  *hap.Server
	mdns *mdns.ServiceEntry
	log  zerolog.Logger

	pairings []string
	conns    []any
	mu       sync.Mutex

	accessory *hap.Accessory
	setupID   string
	stream    string // stream name

	proxyURL string // transparent proxy URL

	// Injected dependencies
	streams    StreamProvider
	store      PairingStore
	snapshots  SnapshotProvider
	liveStream LiveStreamHandler

	// HKSV fields
	motionMode       string
	motionThreshold  float64
	motionDetector   *MotionDetector
	hksvSession      *hksvSession
	continuousMotion bool
	preparedConsumer *HKSVConsumer
}

// NewServer creates a new HKSV server with the given configuration.
func NewServer(cfg Config) (*Server, error) {
	if cfg.Pin == "" {
		cfg.Pin = "27041991"
	}

	pin, err := hap.SanitizePin(cfg.Pin)
	if err != nil {
		return nil, fmt.Errorf("hksv: invalid pin: %w", err)
	}

	deviceID := CalcDeviceID(cfg.DeviceID, cfg.StreamName)
	name := CalcName(cfg.Name, deviceID)
	setupID := CalcSetupID(cfg.StreamName)

	srv := &Server{
		stream:          cfg.StreamName,
		pairings:        cfg.Pairings,
		setupID:         setupID,
		log:             cfg.Logger,
		streams:         cfg.Streams,
		store:           cfg.Store,
		snapshots:       cfg.Snapshots,
		liveStream:      cfg.LiveStream,
		motionMode:      cfg.MotionMode,
		motionThreshold: cfg.MotionThreshold,
	}

	srv.hap = &hap.Server{
		Pin:             pin,
		DeviceID:        deviceID,
		DevicePrivate:   CalcDevicePrivate(cfg.DevicePrivate, cfg.StreamName),
		GetClientPublic: srv.GetPair,
	}

	categoryID := CalcCategoryID(cfg.CategoryID)

	srv.mdns = &mdns.ServiceEntry{
		Name: name,
		Port: cfg.Port,
		Info: map[string]string{
			hap.TXTConfigNumber: "1",
			hap.TXTFeatureFlags: "0",
			hap.TXTDeviceID:     deviceID,
			hap.TXTModel:        cfg.UserAgent,
			hap.TXTProtoVersion: "1.1",
			hap.TXTStateNumber:  "1",
			hap.TXTStatusFlags:  hap.StatusNotPaired,
			hap.TXTCategory:     categoryID,
			hap.TXTSetupHash:    hap.SetupHash(setupID, deviceID),
		},
	}

	srv.UpdateStatus()

	if cfg.ProxyURL != "" {
		// Proxy mode: no local accessory
		srv.proxyURL = cfg.ProxyURL
	} else if cfg.HKSV {
		if srv.motionThreshold <= 0 {
			srv.motionThreshold = defaultThreshold
		}
		srv.log.Debug().Str("stream", cfg.StreamName).Str("motion", cfg.MotionMode).
			Float64("threshold", srv.motionThreshold).Msg("[hksv] HKSV mode")

		if cfg.CategoryID == "doorbell" {
			srv.accessory = camera.NewHKSVDoorbellAccessory("AlexxIT", "go2rtc", name, "-", cfg.Version)
		} else {
			srv.accessory = camera.NewHKSVAccessory("AlexxIT", "go2rtc", name, "-", cfg.Version)
		}
	} else {
		srv.accessory = camera.NewAccessory("AlexxIT", "go2rtc", name, "-", cfg.Version)
	}

	// Remove Speaker service unless explicitly enabled (default: disabled)
	if (cfg.Speaker == nil || !*cfg.Speaker) && srv.accessory != nil {
		filtered := srv.accessory.Services[:0]
		for _, svc := range srv.accessory.Services {
			if svc.Type != "113" { // 113 = Speaker
				filtered = append(filtered, svc)
			}
		}
		srv.accessory.Services = filtered
		srv.accessory.InitIID() // recalculate IIDs
	}

	return srv, nil
}

// MDNSEntry returns the mDNS service entry for advertisement.
func (s *Server) MDNSEntry() *mdns.ServiceEntry {
	return s.mdns
}

// Accessory returns the HAP accessory.
func (s *Server) Accessory() *hap.Accessory {
	return s.accessory
}

// StreamName returns the configured stream name.
func (s *Server) StreamName() string {
	return s.stream
}

func (s *Server) MarshalJSON() ([]byte, error) {
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

// Handle processes an incoming HAP connection (called from your HTTP server).
func (s *Server) Handle(w http.ResponseWriter, r *http.Request) {
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
			s.log.Error().Err(err).Caller().Send()
			return
		}

		s.AddPair(id, key, hap.PermissionAdmin)

	case hap.PathPairVerify:
		id, key, err := s.hap.PairVerify(r, rw)
		if err != nil {
			s.log.Debug().Err(err).Caller().Send()
			return
		}

		s.log.Debug().Str("stream", s.stream).Str("client_id", id).Msgf("[hksv] %s: new conn", conn.RemoteAddr())

		controller, err := hap.NewConn(conn, rw, key, false)
		if err != nil {
			s.log.Error().Err(err).Caller().Send()
			return
		}

		s.AddConn(controller)
		defer s.DelConn(controller)

		// start motion on first Home Hub connection
		switch s.motionMode {
		case "detect":
			go s.startMotionDetector()
		case "continuous":
			go s.prepareHKSVConsumer()
			go s.startContinuousMotion()
		}

		var handler homekit.HandlerFunc

		switch {
		case s.accessory != nil:
			handler = homekit.ServerHandler(s)
		case s.proxyURL != "":
			client, err := hap.Dial(s.proxyURL)
			if err != nil {
				s.log.Error().Err(err).Caller().Send()
				return
			}
			handler = homekit.ProxyHandler(s, client.Conn)
		}

		s.log.Debug().Str("stream", s.stream).Msgf("[hksv] handler started for %s", conn.RemoteAddr())

		if err = handler(controller); err != nil {
			if errors.Is(err, io.EOF) || isClosedConnErr(err) {
				s.log.Debug().Str("stream", s.stream).Msgf("[hksv] %s: connection closed", conn.RemoteAddr())
			} else {
				s.log.Error().Err(err).Str("stream", s.stream).Caller().Send()
			}
			return
		}
	}
}

// AddConn registers a connection for tracking.
func (s *Server) AddConn(v any) {
	s.log.Trace().Str("stream", s.stream).Msgf("[hksv] add conn %s", connLabel(v))
	s.mu.Lock()
	s.conns = append(s.conns, v)
	s.mu.Unlock()
}

// DelConn unregisters a connection.
func (s *Server) DelConn(v any) {
	s.log.Trace().Str("stream", s.stream).Msgf("[hksv] del conn %s", connLabel(v))
	s.mu.Lock()
	if i := slices.Index(s.conns, v); i >= 0 {
		s.conns = slices.Delete(s.conns, i, i+1)
	}
	s.mu.Unlock()
}

// connLabel returns a short human-readable label for a connection.
func connLabel(v any) string {
	switch v := v.(type) {
	case *hap.Conn:
		return "hap " + v.RemoteAddr().String()
	case *hds.Conn:
		return "hds " + v.RemoteAddr().String()
	}
	if s, ok := v.(fmt.Stringer); ok {
		return s.String()
	}
	return fmt.Sprintf("%T", v)
}

func (s *Server) UpdateStatus() {
	if len(s.pairings) == 0 {
		s.mdns.Info[hap.TXTStatusFlags] = hap.StatusNotPaired
	} else {
		s.mdns.Info[hap.TXTStatusFlags] = hap.StatusPaired
	}
}

func (s *Server) pairIndex(id string) int {
	id = "client_id=" + id
	for i, pairing := range s.pairings {
		if strings.HasPrefix(pairing, id) {
			return i
		}
	}
	return -1
}

func (s *Server) GetPair(id string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	if i := s.pairIndex(id); i >= 0 {
		query, _ := url.ParseQuery(s.pairings[i])
		b, _ := hex.DecodeString(query.Get("client_public"))
		return b
	}
	return nil
}

func (s *Server) AddPair(id string, public []byte, permissions byte) {
	s.log.Debug().Str("stream", s.stream).Msgf("[hksv] add pair id=%s public=%x perm=%d", id, public, permissions)

	s.mu.Lock()
	if s.pairIndex(id) < 0 {
		s.pairings = append(s.pairings, fmt.Sprintf(
			"client_id=%s&client_public=%x&permissions=%d", id, public, permissions,
		))
		s.UpdateStatus()
		s.savePairings()
	}
	s.mu.Unlock()
}

func (s *Server) DelPair(id string) {
	s.log.Debug().Str("stream", s.stream).Msgf("[hksv] del pair id=%s", id)

	s.mu.Lock()
	if i := s.pairIndex(id); i >= 0 {
		s.pairings = append(s.pairings[:i], s.pairings[i+1:]...)
		s.UpdateStatus()
		s.savePairings()
	}
	s.mu.Unlock()
}

func (s *Server) savePairings() {
	if s.store != nil {
		if err := s.store.SavePairings(s.stream, s.pairings); err != nil {
			s.log.Error().Err(err).Msgf("[hksv] can't save %s pairings=%v", s.stream, s.pairings)
		}
	}
}

func (s *Server) GetAccessories(_ net.Conn) []*hap.Accessory {
	s.log.Trace().Str("stream", s.stream).Msg("[hksv] GET /accessories")
	if s.log.Trace().Enabled() {
		if b, err := json.Marshal(s.accessory); err == nil {
			s.log.Trace().Str("stream", s.stream).Str("accessory", string(b)).Msg("[hksv] accessory JSON")
		}
	}
	return []*hap.Accessory{s.accessory}
}

func (s *Server) GetCharacteristic(conn net.Conn, aid uint8, iid uint64) any {
	s.log.Trace().Str("stream", s.stream).Msgf("[hksv] get char aid=%d iid=0x%x", aid, iid)

	char := s.accessory.GetCharacterByID(iid)
	if char == nil {
		s.log.Warn().Msgf("[hksv] get unknown characteristic: %d", iid)
		return nil
	}

	switch char.Type {
	case camera.TypeSetupEndpoints:
		if s.liveStream != nil {
			return s.liveStream.GetEndpointsResponse()
		}
		return nil
	}

	return char.Value
}

func (s *Server) SetCharacteristic(conn net.Conn, aid uint8, iid uint64, value any) {
	s.log.Trace().Str("stream", s.stream).Msgf("[hksv] set char aid=%d iid=0x%x value=%v", aid, iid, value)

	char := s.accessory.GetCharacterByID(iid)
	if char == nil {
		s.log.Warn().Msgf("[hksv] set unknown characteristic: %d", iid)
		return
	}

	switch char.Type {
	case camera.TypeSetupEndpoints:
		if s.liveStream == nil {
			return
		}
		var offer camera.SetupEndpointsRequest
		if err := tlv8.UnmarshalBase64(value, &offer); err != nil {
			return
		}
		resp, err := s.liveStream.SetupEndpoints(conn, &offer)
		if err != nil {
			s.log.Error().Err(err).Msg("[hksv] setup endpoints failed")
			return
		}
		// Keep the latest response in characteristic value for write-response (r=true)
		// and subsequent GET /characteristics reads.
		char.Value = resp

	case camera.TypeSelectedStreamConfiguration:
		if s.liveStream == nil {
			return
		}
		var conf camera.SelectedStreamConfiguration
		if err := tlv8.UnmarshalBase64(value, &conf); err != nil {
			return
		}
		s.log.Trace().Str("stream", s.stream).Msgf("[hksv] stream id=%x cmd=%d", conf.Control.SessionID, conf.Control.Command)

		switch conf.Control.Command {
		case camera.SessionCommandEnd:
			_ = s.liveStream.StopStream(conf.Control.SessionID, s)
		case camera.SessionCommandStart:
			_ = s.liveStream.StartStream(s.stream, &conf, s)
		}

	case camera.TypeSetupDataStreamTransport:
		var req camera.SetupDataStreamTransportRequest
		if err := tlv8.UnmarshalBase64(value, &req); err != nil {
			s.log.Error().Err(err).Str("stream", s.stream).Msg("[hksv] parse ch131 failed")
			return
		}

		s.log.Debug().Str("stream", s.stream).Uint8("cmd", req.SessionCommandType).
			Uint8("transport", req.TransportType).Msg("[hksv] DataStream setup")

		if req.SessionCommandType != 0 {
			s.log.Debug().Str("stream", s.stream).Msg("[hksv] DataStream close request")
			if s.hksvSession != nil {
				s.hksvSession.Close()
			}
			return
		}

		accessoryKeySalt := core.RandString(32, 0)
		combinedSalt := req.ControllerKeySalt + accessoryKeySalt

		ln, err := net.ListenTCP("tcp", nil)
		if err != nil {
			s.log.Error().Err(err).Str("stream", s.stream).Msg("[hksv] listen failed")
			return
		}
		port := ln.Addr().(*net.TCPAddr).Port

		resp := camera.SetupDataStreamTransportResponse{
			Status:           0,
			AccessoryKeySalt: accessoryKeySalt,
		}
		resp.TransportTypeSessionParameters.TCPListeningPort = uint16(port)

		v, err := tlv8.MarshalBase64(resp)
		if err != nil {
			ln.Close()
			return
		}
		char.Value = v

		s.log.Debug().Str("stream", s.stream).Int("port", port).Msg("[hksv] listening for HDS")

		hapConn := conn.(*hap.Conn)
		go s.acceptHDS(hapConn, ln, combinedSalt)

	case camera.TypeSelectedCameraRecordingConfiguration:
		s.log.Debug().Str("stream", s.stream).Str("motion", s.motionMode).Msg("[hksv] selected recording config")
		char.Value = value

		switch s.motionMode {
		case "continuous":
			go s.startContinuousMotion()
		case "detect":
			go s.startMotionDetector()
		}

	default:
		char.Value = value
	}
}

func (s *Server) GetImage(conn net.Conn, width, height int) []byte {
	s.log.Trace().Str("stream", s.stream).Msgf("[hksv] get image width=%d height=%d", width, height)

	if s.snapshots == nil {
		return nil
	}

	b, err := s.snapshots.GetSnapshot(s.stream, width, height)
	if err != nil {
		s.log.Error().Err(err).Msg("[hksv] snapshot failed")
		return nil
	}
	return b
}

// SetMotionDetected triggers or clears the motion detected characteristic.
func (s *Server) SetMotionDetected(detected bool) {
	if s.accessory == nil {
		return
	}
	char := s.accessory.GetCharacter("22") // MotionDetected
	if char == nil {
		return
	}
	char.Value = detected
	_ = char.NotifyListeners(nil)
	s.log.Debug().Str("stream", s.stream).Bool("motion", detected).Msg("[hksv] motion")
}

// MotionDetected returns the current motion detected state.
func (s *Server) MotionDetected() bool {
	if s.accessory == nil {
		return false
	}
	char := s.accessory.GetCharacter("22") // MotionDetected
	if char == nil {
		return false
	}
	v, _ := char.Value.(bool)
	return v
}

// TriggerDoorbell triggers a doorbell press event.
func (s *Server) TriggerDoorbell() {
	if s.accessory == nil {
		return
	}
	char := s.accessory.GetCharacter("73") // ProgrammableSwitchEvent
	if char == nil {
		return
	}
	char.Value = 0 // SINGLE_PRESS
	_ = char.NotifyListeners(nil)
	s.log.Debug().Str("stream", s.stream).Msg("[hksv] doorbell")
}

// acceptHDS opens a TCP listener for the HDS DataStream connection from the Home Hub
func (s *Server) acceptHDS(hapConn *hap.Conn, ln net.Listener, salt string) {
	defer ln.Close()

	if tcpLn, ok := ln.(*net.TCPListener); ok {
		_ = tcpLn.SetDeadline(time.Now().Add(30 * time.Second))
	}

	rawConn, err := ln.Accept()
	if err != nil {
		s.log.Error().Err(err).Str("stream", s.stream).Msg("[hksv] accept failed")
		return
	}
	defer rawConn.Close()

	hdsConn, err := hds.NewConn(rawConn, hapConn.SharedKey, salt, false)
	if err != nil {
		s.log.Error().Err(err).Str("stream", s.stream).Msg("[hksv] hds conn failed")
		return
	}

	s.AddConn(hdsConn)
	defer s.DelConn(hdsConn)

	session := newHKSVSession(s, hapConn, hdsConn)

	s.mu.Lock()
	s.hksvSession = session
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if s.hksvSession == session {
			s.hksvSession = nil
		}
		s.mu.Unlock()
		session.Close()
	}()

	s.log.Debug().Str("stream", s.stream).Msg("[hksv] session started")

	if err := session.Run(); err != nil {
		s.log.Debug().Err(err).Str("stream", s.stream).Msg("[hksv] session ended")
	}
}

// prepareHKSVConsumer pre-starts a consumer and adds it to the stream.
func (s *Server) prepareHKSVConsumer() {
	consumer := NewHKSVConsumer(s.log)

	if err := s.streams.AddConsumer(s.stream, consumer); err != nil {
		s.log.Debug().Err(err).Str("stream", s.stream).Msg("[hksv] prepare consumer failed")
		return
	}

	s.log.Debug().Str("stream", s.stream).Msg("[hksv] consumer prepared")

	s.mu.Lock()
	if s.preparedConsumer != nil {
		old := s.preparedConsumer
		s.preparedConsumer = nil
		s.mu.Unlock()
		s.streams.RemoveConsumer(s.stream, old)
		_ = old.Stop()
		s.mu.Lock()
	}
	s.preparedConsumer = consumer
	s.mu.Unlock()

	// Keep alive until used or timeout (60 seconds)
	select {
	case <-consumer.Done():
		// consumer was stopped (used or server closed)
	case <-time.After(60 * time.Second):
		s.mu.Lock()
		if s.preparedConsumer == consumer {
			s.preparedConsumer = nil
			s.mu.Unlock()
			s.streams.RemoveConsumer(s.stream, consumer)
			_ = consumer.Stop()
			s.log.Debug().Str("stream", s.stream).Msg("[hksv] prepared consumer expired")
		} else {
			s.mu.Unlock()
		}
	}
}

func (s *Server) takePreparedConsumer() *HKSVConsumer {
	s.mu.Lock()
	defer s.mu.Unlock()
	consumer := s.preparedConsumer
	s.preparedConsumer = nil
	return consumer
}

func (s *Server) startMotionDetector() {
	s.mu.Lock()
	if s.motionDetector != nil {
		s.mu.Unlock()
		return
	}
	det := NewMotionDetector(s.motionThreshold, s.SetMotionDetected, s.log)
	s.motionDetector = det
	s.mu.Unlock()

	s.AddConn(det)

	if err := s.streams.AddConsumer(s.stream, det); err != nil {
		s.log.Error().Err(err).Str("stream", s.stream).Msg("[hksv] motion detector add consumer failed")
		s.DelConn(det)
		s.mu.Lock()
		s.motionDetector = nil
		s.mu.Unlock()
		return
	}

	s.log.Debug().Str("stream", s.stream).Msg("[hksv] motion detector started")

	_, _ = det.WriteTo(nil) // blocks until Stop()

	s.streams.RemoveConsumer(s.stream, det)
	s.DelConn(det)

	s.mu.Lock()
	if s.motionDetector == det {
		s.motionDetector = nil
	}
	s.mu.Unlock()

	s.log.Debug().Str("stream", s.stream).Msg("[hksv] motion detector stopped")
}

func (s *Server) stopMotionDetector() {
	s.mu.Lock()
	det := s.motionDetector
	s.mu.Unlock()
	if det != nil {
		_ = det.Stop()
	}
}

func (s *Server) startContinuousMotion() {
	s.mu.Lock()
	if s.continuousMotion {
		s.mu.Unlock()
		return
	}
	s.continuousMotion = true
	s.mu.Unlock()

	s.log.Debug().Str("stream", s.stream).Msg("[hksv] continuous motion started")

	// delay to allow Home Hub to subscribe to events
	time.Sleep(5 * time.Second)

	s.SetMotionDetected(true)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if s.accessory == nil {
			return
		}
		s.SetMotionDetected(true)
	}
}

// isClosedConnErr checks if the error is a "use of closed network connection" error.
// This happens when the remote side (e.g., iPhone) closes the TCP connection.
func isClosedConnErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "use of closed network connection")
}
