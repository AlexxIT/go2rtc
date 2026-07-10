package homekit

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/google/uuid"
	pion "github.com/pion/webrtc/v4"
)

// PeerConnectionFactory creates a pion PeerConnection for HomeKit WebRTC
type PeerConnectionFactory func() (*pion.PeerConnection, error)

// WebRTCSession is one active HomeKit WebRTC streaming session
type WebRTCSession struct {
	ID        string
	Conn      *webrtc.Conn
	CreatedAt time.Time
	SFrame    bool
}

// WebRTCManager tracks concurrent HomeKit WebRTC sessions (min 6 required)
type WebRTCManager struct {
	mu       sync.Mutex
	sessions map[string]*WebRTCSession
	factory  PeerConnectionFactory
	max      int

	// CMAF client cert state
	privKey    *ecdsa.PrivateKey
	clientCert []byte
	caCert     []byte
	keyID      uint64
	keys       map[uint64][]byte
}

// NewWebRTCManager creates a session manager
func NewWebRTCManager(factory PeerConnectionFactory) *WebRTCManager {
	return &WebRTCManager{
		sessions: make(map[string]*WebRTCSession),
		factory:  factory,
		max:      camera.MinConcurrentWebRTCSessions,
		keys:     make(map[uint64][]byte),
	}
}

// ActiveCount returns the number of active WebRTC sessions
func (m *WebRTCManager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

// SolicitOffer creates a new session and returns an SDP offer with ICE candidates
func (m *WebRTCManager) SolicitOffer(sframe bool) (*camera.WebRTCSolicitOfferResponse, error) {
	if m.factory == nil {
		return &camera.WebRTCSolicitOfferResponse{Status: camera.WebRTCSolicitError}, nil
	}

	m.mu.Lock()
	if len(m.sessions) >= m.max {
		m.mu.Unlock()
		return &camera.WebRTCSolicitOfferResponse{Status: camera.WebRTCSolicitError}, nil
	}
	m.mu.Unlock()

	pc, err := m.factory()
	if err != nil {
		return &camera.WebRTCSolicitOfferResponse{Status: camera.WebRTCSolicitError}, nil
	}

	conn := webrtc.NewConn(pc)
	conn.Mode = core.ModePassiveConsumer
	conn.Protocol = "homekit-webrtc"
	conn.FormatName = "homekit/webrtc"

	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecH265},
				{Name: core.CodecH264},
			},
		},
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecOpus, ClockRate: 48000, Channels: 2},
			},
		},
	}

	offer, err := conn.CreateCompleteOffer(medias)
	if err != nil {
		_ = conn.Close()
		return &camera.WebRTCSolicitOfferResponse{Status: camera.WebRTCSolicitError}, nil
	}

	// HAP data fields use raw 16-byte UUID
	u := uuid.New()
	sessionRaw := string(u[:])

	sess := &WebRTCSession{
		ID:        sessionRaw,
		Conn:      conn,
		CreatedAt: time.Now(),
		SFrame:    sframe,
	}

	m.mu.Lock()
	m.sessions[sessionRaw] = sess
	m.mu.Unlock()

	// Collect host candidates already present in the complete offer SDP;
	// AdditionalCandidates can stay empty when candidates are inlined in SDP
	res := &camera.WebRTCSolicitOfferResponse{
		SessionIdentifier: sessionRaw,
		SDPOffer:          offer,
		Status:            camera.WebRTCSolicitSuccess,
	}

	if sframe {
		key := make([]byte, 16)
		_, _ = rand.Read(key)
		res.SFrameConfiguration = camera.SFrameKeyData{
			Key: string(key),
			KID: 1,
		}
	}

	return res, nil
}

// ProvideAnswer applies the controller SDP answer and returns status
func (m *WebRTCManager) ProvideAnswer(req *camera.WebRTCProvideAnswerRequest) *camera.WebRTCProvideAnswerResponse {
	m.mu.Lock()
	sess, ok := m.sessions[req.SessionIdentifier]
	m.mu.Unlock()

	if !ok {
		return &camera.WebRTCProvideAnswerResponse{
			SessionIdentifier: req.SessionIdentifier,
			Status:            camera.WebRTCStatusUnknownSessionIdentifier,
		}
	}

	for _, c := range req.AdditionalCandidates {
		_ = sess.Conn.AddCandidate(c.Candidate)
	}

	if err := sess.Conn.SetAnswer(req.SDPAnswer); err != nil {
		return &camera.WebRTCProvideAnswerResponse{
			SessionIdentifier: req.SessionIdentifier,
			Status:            camera.WebRTCStatusError,
		}
	}

	return &camera.WebRTCProvideAnswerResponse{
		SessionIdentifier: req.SessionIdentifier,
		Status:            camera.WebRTCStatusSuccess,
	}
}

// GetSession returns a session by ID
func (m *WebRTCManager) GetSession(id string) *WebRTCSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// EndSession tears down a WebRTC session
func (m *WebRTCManager) EndSession(id string) *camera.WebRTCStreamingControlResponse {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	if !ok {
		return &camera.WebRTCStreamingControlResponse{
			SessionIdentifier: id,
			Status:            camera.WebRTCStatusUnknownSessionIdentifier,
		}
	}

	_ = sess.Conn.Close()
	return &camera.WebRTCStreamingControlResponse{
		SessionIdentifier: id,
		Status:            camera.WebRTCStatusSuccess,
	}
}

// Reoffer handles renegotiation from the controller
func (m *WebRTCManager) Reoffer(req *camera.WebRTCReofferRequest) *camera.WebRTCReofferResponse {
	m.mu.Lock()
	sess, ok := m.sessions[req.SessionIdentifier]
	m.mu.Unlock()

	if !ok {
		return &camera.WebRTCReofferResponse{
			SessionIdentifier: req.SessionIdentifier,
			Status:            camera.WebRTCStatusUnknownSessionIdentifier,
		}
	}

	// Controller sent a new offer; we answer
	if err := sess.Conn.SetOffer(req.SDPOffer); err != nil {
		return &camera.WebRTCReofferResponse{
			SessionIdentifier: req.SessionIdentifier,
			Status:            camera.WebRTCStatusError,
		}
	}

	answer, err := sess.Conn.GetCompleteAnswer(nil, nil)
	if err != nil {
		return &camera.WebRTCReofferResponse{
			SessionIdentifier: req.SessionIdentifier,
			Status:            camera.WebRTCStatusError,
		}
	}

	return &camera.WebRTCReofferResponse{
		SessionIdentifier: req.SessionIdentifier,
		SDPAnswer:         answer,
		Status:            camera.WebRTCStatusSuccess,
	}
}

// UpdateSession applies SFrame key updates
func (m *WebRTCManager) UpdateSession(req *camera.WebRTCUpdateSessionRequest) *camera.WebRTCUpdateSessionResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[req.SessionIdentifier]; !ok {
		return &camera.WebRTCUpdateSessionResponse{
			SessionIdentifier: req.SessionIdentifier,
			Status:            camera.WebRTCStatusUnknownSessionIdentifier,
		}
	}

	for _, k := range req.ReceiveKeysToAdd {
		m.keys[k.KID] = []byte(k.Key)
	}
	for _, k := range req.ReceiveKIDsToRemove {
		delete(m.keys, k.KID)
	}

	return &camera.WebRTCUpdateSessionResponse{
		SessionIdentifier: req.SessionIdentifier,
		Status:            camera.WebRTCStatusSuccess,
	}
}

// HandleCSR generates a client certificate signing request for CMAF ingest
func (m *WebRTCManager) HandleCSR(nonce []byte) (*camera.CameraClientCSRResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.privKey == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		m.privKey = key
	}

	template := x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "go2rtc-hksv"},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &template, m.privKey)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(nonce)
	sig, err := ecdsa.SignASN1(rand.Reader, m.privKey, sum[:])
	if err != nil {
		return nil, err
	}

	return &camera.CameraClientCSRResponse{
		CSR:            string(csrDER),
		NonceSignature: string(sig),
	}, nil
}

// InstallClientCertificate stores the issued CMAF client certificate
func (m *WebRTCManager) InstallClientCertificate(req *camera.CameraClientCertificateRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clientCert = []byte(req.ClientCertificate)
	m.caCert = []byte(req.CA)
}

// CertificateNeedsUpdate reports whether a new client cert is required
func (m *WebRTCManager) CertificateNeedsUpdate() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.clientCert) == 0
}

// SetKey stores a CMAF content key
func (m *WebRTCManager) SetKey(key []byte, number uint64) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys[number] = key
	m.keyID = number
	return number
}

// KeyID returns the current key identifier
func (m *WebRTCManager) KeyID() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.keyID
}

