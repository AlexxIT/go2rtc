package webrtc

import (
	"bytes"
	"fmt"
	"net"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func TestSessionHandler(t *testing.T) {
	for _, value := range []any{nil, true, "", "unknown", "close-on-disconnect-v1"} {
		t.Run("value="+(&ws.Message{Value: value}).String(), func(t *testing.T) {
			tr := new(ws.Transport)
			var messages []*ws.Message
			tr.OnWrite(func(msg any) error {
				messages = append(messages, msg.(*ws.Message))
				return nil
			})

			err := sessionHandler(tr, &ws.Message{Value: value})
			var bound bool
			tr.WithContext(func(ctx map[any]any) {
				bound, _ = ctx["webrtc_lifetime"].(bool)
			})
			if value == "close-on-disconnect-v1" {
				require.NoError(t, err)
				require.True(t, bound)
				require.Equal(t, []*ws.Message{{Type: "webrtc/session", Value: value}}, messages)
				// A repeated opt-in is harmless; an invalid value cannot undo it.
				require.NoError(t, sessionHandler(tr, &ws.Message{Value: value}))
				require.Error(t, sessionHandler(tr, &ws.Message{Value: "unknown"}))
				tr.WithContext(func(ctx map[any]any) {
					require.Equal(t, true, ctx["webrtc_lifetime"])
				})
			} else {
				require.Error(t, err)
				require.False(t, bound)
				require.Empty(t, messages)
			}
		})
	}
}

// These tests use real Pion peer connections and stream tracks, with no camera,
// STUN server, microphone or other external service.
func TestSessionLifetime(t *testing.T) {
	for _, apiV2 := range []bool{false, true} {
		for _, producer := range []bool{false, true} {
			for _, bound := range []bool{false, true} {
				t.Run(sessionTestName(apiV2, producer, bound), func(t *testing.T) {
					fixture := newSessionFixture(t)
					tr, pc := fixture.offer(t, apiV2, producer, bound, false)
					require.Equal(t, pion.PeerConnectionStateConnected, pc.ConnectionState())
					tr.Close()
					if bound {
						require.Equal(t, pion.PeerConnectionStateClosed, pc.ConnectionState())
					} else {
						require.Equal(t, pion.PeerConnectionStateConnected, pc.ConnectionState())
					}
				})
			}
		}
	}
}

func TestSessionCloseDuringSetup(t *testing.T) {
	fixture := newSessionFixture(t)
	_, pc := fixture.offer(t, false, false, true, true)
	// Transport.OnClose must also close a peer registered after disconnection.
	require.Equal(t, pion.PeerConnectionStateClosed, pc.ConnectionState())
}

func TestSessionCloseIsIsolated(t *testing.T) {
	fixture := newSessionFixture(t)
	first, pc1 := fixture.offer(t, false, true, true, false)
	second, pc2 := fixture.offer(t, false, true, true, false)
	third, pc3 := fixture.offer(t, false, true, false, false)
	first.Close()
	require.Equal(t, pion.PeerConnectionStateClosed, pc1.ConnectionState())
	require.NotEqual(t, pion.PeerConnectionStateClosed, pc2.ConnectionState())
	require.NotEqual(t, pion.PeerConnectionStateClosed, pc3.ConnectionState())
	second.Close()
	require.Equal(t, pion.PeerConnectionStateClosed, pc2.ConnectionState())
	third.Close()
	require.NotEqual(t, pion.PeerConnectionStateClosed, pc3.ConnectionState())
}

type sessionProducer struct{ core.Connection }

func (p *sessionProducer) Start() error { return nil }

type sessionFixture struct {
	name  string
	track *core.Receiver
}

func newSessionFixture(t *testing.T) *sessionFixture {
	t.Helper()
	name := t.Name()
	stream, err := streams.New(name)
	require.NoError(t, err)
	t.Cleanup(func() { streams.Delete(name) })
	media := &core.Media{
		Kind: core.KindAudio, Direction: core.DirectionRecvonly,
		Codecs: []*core.Codec{{Name: core.CodecPCMA, ClockRate: 8000, Channels: 1, PayloadType: 8}},
	}
	prod := &sessionProducer{core.Connection{Medias: []*core.Media{media}}}
	track, err := prod.GetTrack(media, media.Codecs[0])
	require.NoError(t, err)
	stream.AddProducer(prod)
	return &sessionFixture{name: name, track: track}
}

func (f *sessionFixture) offer(t *testing.T, apiV2, producer, bound, closed bool) (*ws.Transport, *pion.PeerConnection) {
	t.Helper()
	settings := pion.SettingEngine{}
	settings.SetNetworkTypes([]pion.NetworkType{pion.NetworkTypeUDP4})
	settings.SetIncludeLoopbackCandidate(true)
	settings.SetIPFilter(func(ip net.IP) bool { return ip.IsLoopback() })
	api := pion.NewAPI(pion.WithSettingEngine(settings))
	client, err := api.NewPeerConnection(pion.Configuration{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	received := make(chan struct{}, 1)
	client.OnTrack(func(track *pion.TrackRemote, _ *pion.RTPReceiver) {
		if _, _, err := track.ReadRTP(); err == nil {
			received <- struct{}{}
		}
	})
	direction := pion.RTPTransceiverDirectionRecvonly
	query := "src"
	if producer {
		direction = pion.RTPTransceiverDirectionSendonly
		query = "dst"
	}
	_, err = client.AddTransceiverFromKind(pion.RTPCodecTypeAudio, pion.RTPTransceiverInit{Direction: direction})
	require.NoError(t, err)
	offer, err := client.CreateOffer(nil)
	require.NoError(t, err)
	clientGathered := pion.GatheringCompletePromise(client)
	require.NoError(t, client.SetLocalDescription(offer))
	select {
	case <-clientGathered:
	case <-time.After(time.Second * 5):
		t.Fatal("client ICE gathering timed out")
	}
	offer = *client.LocalDescription()

	pc, err := api.NewPeerConnection(pion.Configuration{})
	require.NoError(t, err)
	serverGathered := pion.GatheringCompletePromise(pc)
	t.Cleanup(func() { _ = pc.Close() })
	previous := PeerConnection
	PeerConnection = func(bool) (*pion.PeerConnection, error) { return pc, nil }
	defer func() { PeerConnection = previous }()
	tr := &ws.Transport{Request: httptest.NewRequest("GET", "/api/ws?"+query+"="+url.QueryEscape(f.name), nil)}
	tr.OnWrite(func(any) error { return nil })
	if bound {
		require.NoError(t, sessionHandler(tr, &ws.Message{Value: "close-on-disconnect-v1"}))
	}
	if closed {
		tr.Close()
	}
	msg := &ws.Message{Type: "webrtc/offer", Value: offer.SDP}
	if apiV2 {
		msg = &ws.Message{Type: "webrtc", Value: offer}
	}
	err = asyncHandler(tr, msg)
	if !closed {
		require.NoError(t, err)
		select {
		case <-serverGathered:
		case <-time.After(time.Second * 5):
			t.Fatal("server ICE gathering timed out")
		}
		require.NoError(t, client.SetRemoteDescription(*pc.LocalDescription()))
		require.Eventually(t, func() bool {
			return pc.ConnectionState() == pion.PeerConnectionStateConnected
		}, time.Second*5, time.Millisecond*10)
		if !producer {
			// Deliver actual PCMA packets before checking the session lifetime.
			// Stop writing before disconnecting to keep the fixture independent
			// of concurrent source/stream teardown elsewhere in the project.
			ticker := time.NewTicker(time.Millisecond * 20)
			defer ticker.Stop()
			timeout := time.NewTimer(time.Second * 5)
			defer timeout.Stop()
			var sequence uint16
		media:
			for {
				select {
				case <-received:
					break media
				case <-timeout.C:
					t.Fatal("no loopback audio received")
				case <-ticker.C:
					sequence++
					f.track.WriteRTP(&rtp.Packet{
						Header: rtp.Header{Version: 2, PayloadType: 8, SequenceNumber: sequence,
							Timestamp: uint32(sequence) * 160, SSRC: 1},
						Payload: bytes.Repeat([]byte{0xD5}, 160),
					})
				}
			}
		}
	}
	return tr, pc
}

func sessionTestName(apiV2, producer, bound bool) string {
	return fmt.Sprintf("v2=%t/producer=%t/bound=%t", apiV2, producer, bound)
}
