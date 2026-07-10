package homekit

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func testFactory() (*pion.PeerConnection, error) {
	api, err := webrtc.NewAPI()
	if err != nil {
		return nil, err
	}
	return api.NewPeerConnection(pion.Configuration{})
}

func TestWebRTCManagerSolicitAndEnd(t *testing.T) {
	m := NewWebRTCManager(testFactory)

	res, err := m.SolicitOffer(false)
	require.NoError(t, err)
	require.Equal(t, byte(camera.WebRTCSolicitSuccess), res.Status)
	require.NotEmpty(t, res.SessionIdentifier)
	require.Contains(t, res.SDPOffer, "v=0")
	require.Equal(t, 1, m.ActiveCount())

	// Unknown session end
	end := m.EndSession("unknown")
	require.Equal(t, byte(camera.WebRTCStatusUnknownSessionIdentifier), end.Status)

	// End real session
	end = m.EndSession(res.SessionIdentifier)
	require.Equal(t, byte(camera.WebRTCStatusSuccess), end.Status)
	require.Equal(t, 0, m.ActiveCount())
}

func TestWebRTCManagerSFrame(t *testing.T) {
	m := NewWebRTCManager(testFactory)
	res, err := m.SolicitOffer(true)
	require.NoError(t, err)
	require.Equal(t, byte(camera.WebRTCSolicitSuccess), res.Status)
	require.NotEmpty(t, res.SFrameConfiguration.Key)
	require.Equal(t, uint64(1), res.SFrameConfiguration.KID)
	_ = m.EndSession(res.SessionIdentifier)
}

func TestWebRTCManagerMaxSessions(t *testing.T) {
	m := NewWebRTCManager(testFactory)
	m.max = 2

	r1, err := m.SolicitOffer(false)
	require.NoError(t, err)
	require.Equal(t, byte(camera.WebRTCSolicitSuccess), r1.Status)

	r2, err := m.SolicitOffer(false)
	require.NoError(t, err)
	require.Equal(t, byte(camera.WebRTCSolicitSuccess), r2.Status)

	r3, err := m.SolicitOffer(false)
	require.NoError(t, err)
	require.Equal(t, byte(camera.WebRTCSolicitError), r3.Status)

	_ = m.EndSession(r1.SessionIdentifier)
	_ = m.EndSession(r2.SessionIdentifier)
}

func TestWebRTCManagerNilFactory(t *testing.T) {
	m := NewWebRTCManager(nil)
	res, err := m.SolicitOffer(false)
	require.NoError(t, err)
	require.Equal(t, byte(camera.WebRTCSolicitError), res.Status)
}

func TestCSRGeneration(t *testing.T) {
	m := NewWebRTCManager(nil)
	nonce := make([]byte, 32)
	_, _ = rand.Read(nonce)

	res, err := m.HandleCSR(nonce)
	require.NoError(t, err)
	require.NotEmpty(t, res.CSR)
	require.NotEmpty(t, res.NonceSignature)
	require.True(t, m.CertificateNeedsUpdate())

	// Install cert
	m.InstallClientCertificate(&camera.CameraClientCertificateRequest{
		ClientCertificate: "client-der",
		CA:                "ca-der",
	})
	require.False(t, m.CertificateNeedsUpdate())
}

func TestKeyManagement(t *testing.T) {
	m := NewWebRTCManager(nil)
	id := m.SetKey([]byte("key-data"), 7)
	require.Equal(t, uint64(7), id)
	require.Equal(t, uint64(7), m.KeyID())
}

func TestUpdateSessionKeys(t *testing.T) {
	m := NewWebRTCManager(testFactory)
	res, err := m.SolicitOffer(false)
	require.NoError(t, err)

	upd := m.UpdateSession(&camera.WebRTCUpdateSessionRequest{
		SessionIdentifier: res.SessionIdentifier,
		ReceiveKeysToAdd: []camera.SFrameKeyData{
			{Key: "abc", KID: 9},
		},
	})
	require.Equal(t, byte(camera.WebRTCStatusSuccess), upd.Status)

	upd = m.UpdateSession(&camera.WebRTCUpdateSessionRequest{
		SessionIdentifier: "missing",
	})
	require.Equal(t, byte(camera.WebRTCStatusUnknownSessionIdentifier), upd.Status)

	_ = m.EndSession(res.SessionIdentifier)
}

func TestProvideAnswerUnknown(t *testing.T) {
	m := NewWebRTCManager(testFactory)
	res := m.ProvideAnswer(&camera.WebRTCProvideAnswerRequest{
		SessionIdentifier: "nope",
		SDPAnswer:         "v=0\r\n",
	})
	require.Equal(t, byte(camera.WebRTCStatusUnknownSessionIdentifier), res.Status)
}

func TestECDSAKeyGen(t *testing.T) {
	// Sanity: P-256 works on this platform (used by CSR)
	_, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
}
