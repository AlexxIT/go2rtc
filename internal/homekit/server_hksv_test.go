package homekit

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	pkghk "github.com/AlexxIT/go2rtc/pkg/homekit"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func testPCFactory() (*pion.PeerConnection, error) {
	api, err := webrtc.NewAPI()
	if err != nil {
		return nil, err
	}
	return api.NewPeerConnection(pion.Configuration{})
}

func newTestHKSVServer(t *testing.T) *server {
	t.Helper()
	acc := camera.NewHKSVAccessory("AlexxIT", "go2rtc", "test-cam", "SN", "1.0", "seed-test")
	s := &server{
		stream:    "test-cam",
		accessory: acc,
		hksv:      true,
		webrtc:    pkghk.NewWebRTCManager(testPCFactory),
		recording: pkghk.NewRecordingManager(),
		wrValues:  map[uint64]any{},
	}
	s.recording.OnEventSeq = s.notifyEventSequence
	return s
}

func (s *server) mustIID(t *testing.T, typ string) uint64 {
	t.Helper()
	ch := s.accessory.GetCharacter(typ)
	require.NotNil(t, ch, "missing characteristic type %s", typ)
	require.NotZero(t, ch.IID)
	return ch.IID
}

func mustTLV8B64(t *testing.T, v any) string {
	t.Helper()
	s, err := tlv8.MarshalBase64(v)
	require.NoError(t, err)
	return s
}

func TestServerCSRAndCertificateFlow(t *testing.T) {
	s := newTestHKSVServer(t)
	csrIID := s.mustIID(t, camera.TypeCameraClientCSR)
	statusIID := s.mustIID(t, camera.TypeCameraClientCertificateStatus)
	certIID := s.mustIID(t, camera.TypeCameraClientCertificate)

	// Before cert: NeedsUpdate true
	raw := s.GetCharacteristic(nil, 1, statusIID)
	require.NotNil(t, raw)
	var status camera.CameraClientCertificateStatusValue
	require.NoError(t, tlv8.UnmarshalBase64(raw, &status))
	require.True(t, status.NeedsUpdate)

	nonce := make([]byte, 32)
	_, err := rand.Read(nonce)
	require.NoError(t, err)

	s.SetCharacteristic(nil, 1, csrIID, mustTLV8B64(t, camera.CameraClientCSRRequest{
		Nonce: string(nonce),
	}))

	// write-response stored
	wr := s.GetCharacteristic(nil, 1, csrIID)
	require.NotNil(t, wr)
	var csrRes camera.CameraClientCSRResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &csrRes))
	require.NotEmpty(t, csrRes.CSR)
	require.NotEmpty(t, csrRes.NonceSignature)

	csr, err := x509.ParseCertificateRequest([]byte(csrRes.CSR))
	require.NoError(t, err)
	require.NoError(t, csr.CheckSignature())

	// issue leaf matching CSR pubkey
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "go2rtc-hksv"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, template, template, csr.PublicKey, caKey)
	require.NoError(t, err)

	s.SetCharacteristic(nil, 1, certIID, mustTLV8B64(t, camera.CameraClientCertificateRequest{
		ClientCertificate: string(leafDER),
		CA:                string(leafDER),
	}))
	require.False(t, s.recording.Creds.NeedsUpdate())

	// status characteristic should reflect installed cert
	// clear wr cache path by reading status type via GetCharacter path
	// GetCharacteristic for status rebuilds from creds
	raw = s.GetCharacteristic(nil, 1, statusIID)
	// may hit wrValues if we never wrote status; force by type switch
	// TypeCameraClientCertificateStatus is handled specially
	require.NoError(t, tlv8.UnmarshalBase64(raw, &status))
	require.False(t, status.NeedsUpdate)
}

func TestServerKeyAndPublishingPoint(t *testing.T) {
	s := newTestHKSVServer(t)
	keyIID := s.mustIID(t, camera.TypeCameraKey)
	keyIDIID := s.mustIID(t, camera.TypeCameraKeyID)
	pubIID := s.mustIID(t, camera.TypeCameraRecordingPublishingPoint)

	s.SetCharacteristic(nil, 1, keyIID, mustTLV8B64(t, camera.CameraKeyValue{
		Key:       "0123456789abcdef",
		KeyNumber: 7,
	}))
	id, key := s.recording.Creds.CurrentKey()
	require.Equal(t, uint64(7), id)
	require.Equal(t, []byte("0123456789abcdef"), key)

	// KeyID characteristic updated
	ch := s.accessory.GetCharacterByID(keyIDIID)
	require.NotNil(t, ch)
	var kid camera.CameraKeyIDValue
	require.NoError(t, ch.ReadTLV8(&kid))
	require.Equal(t, uint64(7), kid.KeyID)

	s.SetCharacteristic(nil, 1, pubIID, mustTLV8B64(t, camera.CameraRecordingPublishingPointValue{
		URL: "https://example.apple.com/cmaf/ingest/",
		ServerCACertificates: []camera.Certificate{
			{Certificate: "ca-der-bytes"},
		},
	}))
	require.Equal(t, "https://example.apple.com/cmaf/ingest/", s.recording.Creds.PublishingPoint())
}

func TestServerRecordingActiveFlags(t *testing.T) {
	s := newTestHKSVServer(t)
	// Recording Management Active is the second Active char; set by IID from service
	recSvc := s.accessory.GetService(camera.TypeCameraRecordingManagement)
	require.NotNil(t, recSvc)
	var activeIID, audioIID uint64
	for _, ch := range recSvc.Characters {
		switch ch.Type {
		case camera.TypeActive:
			activeIID = ch.IID
		case camera.TypeRecordingAudioActive:
			audioIID = ch.IID
		}
	}
	require.NotZero(t, activeIID)
	require.NotZero(t, audioIID)

	s.SetCharacteristic(nil, 1, activeIID, float64(1))
	require.True(t, s.recording.RecordingActive())
	require.Equal(t, uint8(1), s.accessory.GetCharacterByID(activeIID).Value)

	s.SetCharacteristic(nil, 1, audioIID, float64(1))
	require.True(t, s.recording.AudioActive())

	s.SetCharacteristic(nil, 1, activeIID, float64(0))
	require.False(t, s.recording.RecordingActive())
}

func TestServerBufferActivityAndEvents(t *testing.T) {
	s := newTestHKSVServer(t)
	actIID := s.mustIID(t, camera.TypeBufferActivityCommand)
	evtIID := s.mustIID(t, camera.TypeBufferEventCommand)
	seqIID := s.mustIID(t, camera.TypeBufferEventSequenceNumber)

	require.Equal(t, uint32(0), s.GetCharacteristic(nil, 1, seqIID))

	s.SetCharacteristic(nil, 1, actIID, mustTLV8B64(t, camera.BufferActivityCommandRequest{
		Activity: camera.BufferActivityShouldRecord,
	}))
	require.Equal(t, uint32(1), s.GetCharacteristic(nil, 1, seqIID))

	// sequence characteristic also updated via notify
	ch := s.accessory.GetCharacterByID(seqIID)
	require.Equal(t, uint32(1), ch.Value)

	s.SetCharacteristic(nil, 1, evtIID, mustTLV8B64(t, camera.BufferEventCommandRequest{
		Command:        camera.BufferEventQuery,
		SequenceNumber: 0,
		Limit:          10,
	}))
	wr := s.GetCharacteristic(nil, 1, evtIID)
	var res camera.BufferEventCommandResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &res))
	require.Len(t, res.Events, 1)
	require.Equal(t, byte(camera.BufferEventTypeMotion), res.Events[0].Type)
	require.True(t, res.Events[0].Motion.Active)

	s.SetCharacteristic(nil, 1, evtIID, mustTLV8B64(t, camera.BufferEventCommandRequest{
		Command:        camera.BufferEventAcknowledge,
		SequenceNumber: 1,
	}))
	s.SetCharacteristic(nil, 1, evtIID, mustTLV8B64(t, camera.BufferEventCommandRequest{
		Command: camera.BufferEventQuery,
		Limit:   10,
	}))
	wr = s.GetCharacteristic(nil, 1, evtIID)
	var res2 camera.BufferEventCommandResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &res2))
	require.Empty(t, res2.Events)
}

func TestServerBufferUploadEndToEnd(t *testing.T) {
	var posts atomic.Int32
	srvHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		_, _ = io.ReadAll(r.Body)
		_ = r.Body.Close()
		w.WriteHeader(http.StatusOK)
	}))
	defer srvHTTP.Close()

	s := newTestHKSVServer(t)
	pubIID := s.mustIID(t, camera.TypeCameraRecordingPublishingPoint)
	upIID := s.mustIID(t, camera.TypeBufferUploadCommand)
	evtIID := s.mustIID(t, camera.TypeBufferEventCommand)

	s.SetCharacteristic(nil, 1, pubIID, mustTLV8B64(t, camera.CameraRecordingPublishingPointValue{
		URL: srvHTTP.URL + "/cmaf/",
	}))

	// seed ring buffer with a tiny HEVC-like frame
	codec := &core.Codec{Name: core.CodecH265, ClockRate: 90000}
	nal := []byte{0x40, 0x01, 0x0c, 0x01}
	payload := append([]byte{0, 0, 0, byte(len(nal))}, nal...)
	now := time.Now()
	for i := 0; i < 4; i++ {
		s.recording.Buffer.Push(pkghk.Packet{
			Track:   0,
			Codec:   codec,
			Payload: payload,
			RTPTime: uint32(i * 3000),
			Wall:    now.Add(time.Duration(i) * 100 * time.Millisecond),
			Key:     i == 0,
		})
	}

	s.SetCharacteristic(nil, 1, upIID, mustTLV8B64(t, camera.BufferUploadCommandRequest{
		SessionID: 42,
		Command:   camera.BufferUploadStartAndStop,
		Start:     pkghk.TimeToNTP(now.Add(-time.Second)),
		Stop:      pkghk.TimeToNTP(now.Add(2 * time.Second)),
	}))

	wr := s.GetCharacteristic(nil, 1, upIID)
	var upRes camera.BufferUploadCommandResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &upRes))
	require.NotZero(t, upRes.ClipID)

	// wait for async publish + events
	deadline := time.Now().Add(3 * time.Second)
	var sawStart, sawStop bool
	for time.Now().Before(deadline) {
		s.SetCharacteristic(nil, 1, evtIID, mustTLV8B64(t, camera.BufferEventCommandRequest{
			Command: camera.BufferEventQuery,
			Limit:   32,
		}))
		wr = s.GetCharacteristic(nil, 1, evtIID)
		var evRes camera.BufferEventCommandResponse
		require.NoError(t, tlv8.UnmarshalBase64(wr, &evRes))
		for _, ev := range evRes.Events {
			if ev.Type == camera.BufferEventTypeCMAFSessionStart {
				sawStart = true
				require.Equal(t, uint64(42), ev.CMAFSessionStart.CMAFSessionID)
			}
			if ev.Type == camera.BufferEventTypeCMAFSessionStop {
				sawStop = true
			}
			require.NotEqual(t, byte(camera.BufferEventTypeCMAFError), ev.Type, "unexpected cmaf error event %+v", ev)
		}
		if sawStart && sawStop && posts.Load() > 0 {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	require.True(t, sawStart, "missing session start event")
	require.True(t, sawStop, "missing session stop event")
	require.Greater(t, posts.Load(), int32(0), "expected CMAF POSTs to ingest server")
}

func TestServerWebRTCSolicitAndSessionCount(t *testing.T) {
	s := newTestHKSVServer(t)
	solicitIID := s.mustIID(t, camera.TypeWebRTCSolicitOffer)
	countIID := s.mustIID(t, camera.TypeWebRTCNumberOfActiveSessions)
	ctrlIID := s.mustIID(t, camera.TypeWebRTCStreamingControl)

	require.Equal(t, 0, s.GetCharacteristic(nil, 1, countIID))

	s.SetCharacteristic(nil, 1, solicitIID, mustTLV8B64(t, camera.WebRTCSolicitOfferRequest{
		Options: camera.WebRTCOfferOptions{SFrameEnabled: false},
	}))
	wr := s.GetCharacteristic(nil, 1, solicitIID)
	var res camera.WebRTCSolicitOfferResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &res))
	require.Equal(t, byte(camera.WebRTCSolicitSuccess), res.Status)
	require.NotEmpty(t, res.SessionIdentifier)
	require.Contains(t, res.SDPOffer, "v=0")
	require.Equal(t, 1, s.GetCharacteristic(nil, 1, countIID))

	// end session
	s.SetCharacteristic(nil, 1, ctrlIID, mustTLV8B64(t, camera.WebRTCStreamingControlRequest{
		SessionIdentifier: res.SessionIdentifier,
		Command:           camera.WebRTCCommandEnd,
	}))
	wr = s.GetCharacteristic(nil, 1, ctrlIID)
	var end camera.WebRTCStreamingControlResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &end))
	require.Equal(t, byte(camera.WebRTCStatusSuccess), end.Status)
	require.Equal(t, 0, s.GetCharacteristic(nil, 1, countIID))
}

func TestServerWebRTCPrivacyModeBlocksSolicit(t *testing.T) {
	s := newTestHKSVServer(t)
	solicitIID := s.mustIID(t, camera.TypeWebRTCSolicitOffer)

	// flip HomeKit Camera Active off
	active := s.accessory.GetCharacter(camera.TypeHomeKitCameraActive)
	require.NotNil(t, active)
	s.SetCharacteristic(nil, 1, active.IID, false)

	s.SetCharacteristic(nil, 1, solicitIID, mustTLV8B64(t, camera.WebRTCSolicitOfferRequest{}))
	wr := s.GetCharacteristic(nil, 1, solicitIID)
	var res camera.WebRTCSolicitOfferResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &res))
	require.Equal(t, byte(camera.WebRTCSolicitPrivacyModeActive), res.Status)
}

func TestServerWebRTCNilManager(t *testing.T) {
	s := newTestHKSVServer(t)
	s.webrtc = nil
	solicitIID := s.mustIID(t, camera.TypeWebRTCSolicitOffer)
	s.SetCharacteristic(nil, 1, solicitIID, mustTLV8B64(t, camera.WebRTCSolicitOfferRequest{}))
	wr := s.GetCharacteristic(nil, 1, solicitIID)
	var res camera.WebRTCSolicitOfferResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &res))
	require.Equal(t, byte(camera.WebRTCSolicitError), res.Status)
}

func TestServerWebRTCProvideAnswerUnknown(t *testing.T) {
	s := newTestHKSVServer(t)
	ansIID := s.mustIID(t, camera.TypeWebRTCProvideAnswer)
	s.SetCharacteristic(nil, 1, ansIID, mustTLV8B64(t, camera.WebRTCProvideAnswerRequest{
		SessionIdentifier: string(make([]byte, 16)),
		SDPAnswer:         "v=0\r\n",
	}))
	wr := s.GetCharacteristic(nil, 1, ansIID)
	var res camera.WebRTCProvideAnswerResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &res))
	require.Equal(t, byte(camera.WebRTCStatusUnknownSessionIdentifier), res.Status)
}

func TestServerWebRTCReofferAndUpdate(t *testing.T) {
	s := newTestHKSVServer(t)
	solicitIID := s.mustIID(t, camera.TypeWebRTCSolicitOffer)
	reofferIID := s.mustIID(t, camera.TypeWebRTCReoffer)
	updateIID := s.mustIID(t, camera.TypeWebRTCUpdateSession)

	s.SetCharacteristic(nil, 1, solicitIID, mustTLV8B64(t, camera.WebRTCSolicitOfferRequest{}))
	wr := s.GetCharacteristic(nil, 1, solicitIID)
	var offer camera.WebRTCSolicitOfferResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &offer))
	require.Equal(t, byte(camera.WebRTCSolicitSuccess), offer.Status)

	// reoffer with garbage SDP should error (not crash)
	s.SetCharacteristic(nil, 1, reofferIID, mustTLV8B64(t, camera.WebRTCReofferRequest{
		SessionIdentifier: offer.SessionIdentifier,
		SDPOffer:          "not-an-sdp",
	}))
	wr = s.GetCharacteristic(nil, 1, reofferIID)
	var re camera.WebRTCReofferResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &re))
	require.Equal(t, byte(camera.WebRTCStatusError), re.Status)

	s.SetCharacteristic(nil, 1, updateIID, mustTLV8B64(t, camera.WebRTCUpdateSessionRequest{
		SessionIdentifier: offer.SessionIdentifier,
		ReceiveKeysToAdd: []camera.SFrameKeyData{
			{Key: "abc", KID: 3},
		},
	}))
	wr = s.GetCharacteristic(nil, 1, updateIID)
	var up camera.WebRTCUpdateSessionResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &up))
	require.Equal(t, byte(camera.WebRTCStatusSuccess), up.Status)

	// cleanup
	ctrlIID := s.mustIID(t, camera.TypeWebRTCStreamingControl)
	s.SetCharacteristic(nil, 1, ctrlIID, mustTLV8B64(t, camera.WebRTCStreamingControlRequest{
		SessionIdentifier: offer.SessionIdentifier,
		Command:           camera.WebRTCCommandEnd,
	}))
}

func TestServerRTPStreamingControl(t *testing.T) {
	s := newTestHKSVServer(t)
	iid := s.mustIID(t, camera.TypeRTPStreamingControl)
	s.SetCharacteristic(nil, 1, iid, mustTLV8B64(t, camera.RTPStreamingControlRequest{
		SessionIdentifier: "sess",
		Command:           camera.RTPStreamCommandStart,
	}))
	wr := s.GetCharacteristic(nil, 1, iid)
	var res camera.RTPStreamingControlResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &res))
	require.Equal(t, byte(camera.StreamStatusSuccess), res.Status)

	// privacy blocks start
	active := s.accessory.GetCharacter(camera.TypeHomeKitCameraActive)
	s.SetCharacteristic(nil, 1, active.IID, false)
	s.SetCharacteristic(nil, 1, iid, mustTLV8B64(t, camera.RTPStreamingControlRequest{
		SessionIdentifier: "sess2",
		Command:           camera.RTPStreamCommandStart,
	}))
	wr = s.GetCharacteristic(nil, 1, iid)
	require.NoError(t, tlv8.UnmarshalBase64(wr, &res))
	require.Equal(t, byte(camera.StreamStatusError), res.Status)
}

func TestServerInvalidTLV8DoesNotPanic(t *testing.T) {
	s := newTestHKSVServer(t)
	types := []string{
		camera.TypeCameraClientCSR,
		camera.TypeCameraClientCertificate,
		camera.TypeCameraKey,
		camera.TypeCameraRecordingPublishingPoint,
		camera.TypeBufferActivityCommand,
		camera.TypeBufferUploadCommand,
		camera.TypeBufferEventCommand,
		camera.TypeWebRTCSolicitOffer,
		camera.TypeWebRTCProvideAnswer,
		camera.TypeWebRTCStreamingControl,
		camera.TypeWebRTCReoffer,
		camera.TypeWebRTCUpdateSession,
		camera.TypeRTPStreamingControl,
	}
	for _, typ := range types {
		iid := s.mustIID(t, typ)
		require.NotPanics(t, func() {
			s.SetCharacteristic(nil, 1, iid, "%%%not-base64%%%")
			s.SetCharacteristic(nil, 1, iid, mustTLV8B64(t, struct{}{})) // empty/wrong shape
		})
	}
	// unknown iid
	require.NotPanics(t, func() {
		s.SetCharacteristic(nil, 1, 0xdeadbeef, "x")
		_ = s.GetCharacteristic(nil, 1, 0xdeadbeef)
	})
}

func TestServerStreamingEnabledGate(t *testing.T) {
	s := newTestHKSVServer(t)
	// any Streaming Enabled false blocks
	var streamIID uint64
	for _, srv := range s.accessory.Services {
		for _, ch := range srv.Characters {
			if ch.Type == camera.TypeStreamingEnabled {
				streamIID = ch.IID
				break
			}
		}
		if streamIID != 0 {
			break
		}
	}
	require.NotZero(t, streamIID)
	s.SetCharacteristic(nil, 1, streamIID, false)
	require.False(t, s.streamingAllowed())

	s.SetCharacteristic(nil, 1, streamIID, true)
	// HomeKit camera active still true by default
	require.True(t, s.streamingAllowed())
}

func TestServerGetAccessories(t *testing.T) {
	s := newTestHKSVServer(t)
	accs := s.GetAccessories(nil)
	require.Len(t, accs, 1)
	require.Equal(t, s.accessory, accs[0])
	// unique IIDs already covered in camera package; sanity check webrtc service present
	require.NotNil(t, s.accessory.GetService(camera.TypeCameraWebRTCStreamManagement))
	require.NotNil(t, s.accessory.GetService(camera.TypeCameraBufferManagement))
}

func TestServerUploadWithoutRecordingManager(t *testing.T) {
	s := newTestHKSVServer(t)
	s.recording = nil
	upIID := s.mustIID(t, camera.TypeBufferUploadCommand)
	s.SetCharacteristic(nil, 1, upIID, mustTLV8B64(t, camera.BufferUploadCommandRequest{
		SessionID: 1,
		Command:   camera.BufferUploadStartAndStop,
	}))
	wr := s.GetCharacteristic(nil, 1, upIID)
	var res camera.BufferUploadCommandResponse
	require.NoError(t, tlv8.UnmarshalBase64(wr, &res))
	require.Equal(t, uint64(0), res.ClipID)
}

func TestServerConcurrentCharacteristicAccess(t *testing.T) {
	s := newTestHKSVServer(t)
	actIID := s.mustIID(t, camera.TypeBufferActivityCommand)
	seqIID := s.mustIID(t, camera.TypeBufferEventSequenceNumber)
	solicitIID := s.mustIID(t, camera.TypeWebRTCSolicitOffer)
	ctrlIID := s.mustIID(t, camera.TypeWebRTCStreamingControl)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.SetCharacteristic(nil, 1, actIID, mustTLV8B64(t, camera.BufferActivityCommandRequest{
				Activity: camera.BufferActivityShouldRecord,
			}))
			_ = s.GetCharacteristic(nil, 1, seqIID)
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.SetCharacteristic(nil, 1, solicitIID, mustTLV8B64(t, camera.WebRTCSolicitOfferRequest{}))
			wr := s.GetCharacteristic(nil, 1, solicitIID)
			var res camera.WebRTCSolicitOfferResponse
			_ = tlv8.UnmarshalBase64(wr, &res)
			if res.SessionIdentifier != "" {
				s.SetCharacteristic(nil, 1, ctrlIID, mustTLV8B64(t, camera.WebRTCStreamingControlRequest{
					SessionIdentifier: res.SessionIdentifier,
					Command:           camera.WebRTCCommandEnd,
				}))
			}
		}()
	}
	wg.Wait()
	require.Greater(t, s.recording.EventSequence(), uint32(0))
}

func TestTruthy(t *testing.T) {
	require.True(t, truthy(true))
	require.True(t, truthy(float64(1)))
	require.True(t, truthy(1))
	require.True(t, truthy(uint8(1)))
	require.True(t, truthy("1"))
	require.True(t, truthy("true"))
	require.False(t, truthy(false))
	require.False(t, truthy(float64(0)))
	require.False(t, truthy(0))
	require.False(t, truthy(""))
	require.False(t, truthy(nil))
}
