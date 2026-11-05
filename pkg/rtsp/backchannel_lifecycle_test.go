package rtsp

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/stretchr/testify/require"
)

func TestBackchannelRestoredSessionCanStart(t *testing.T) {
	for _, failNewMedia := range []bool{false, true} {
		t.Run(fmt.Sprint("failed_new_media=", failNewMedia), func(t *testing.T) {
			client, sessions, setups, plays := newBackchannelLifecycleClient(t)
			media := client.Medias[0]
			require.NoError(t, client.AddTrack(media, media.Codecs[0], core.NewReceiver(media, media.Codecs[0])))
			if failNewMedia {
				client.state = StatePlay
				other := client.Medias[1]
				require.Error(t, client.AddTrack(other, other.Codecs[0], core.NewReceiver(other, other.Codecs[0])))
			} else {
				require.NoError(t, client.Reconnect())
			}
			require.Equal(t, StateConn, client.state)
			require.Equal(t, byte(10), client.setupChannels[media])
			before := setups.Load()
			require.NoError(t, client.AddTrack(media, media.Codecs[0], core.NewReceiver(media, media.Codecs[0])))
			require.Equal(t, StateSetup, client.state, "restored backchannel must be playable")
			require.Equal(t, before, setups.Load(), "restored media must not repeat SETUP")
			require.Equal(t, int32(2), sessions.Load())
			// The loopback server closes after acknowledging PLAY.
			require.ErrorContains(t, client.Start(), "EOF")
			require.Equal(t, int32(1), plays.Load(), "Start must reach the wire-level PLAY")
		})
	}
}

func TestBackchannelTeardownInvalidatesSession(t *testing.T) {
	client, sessions, setups, plays := newBackchannelLifecycleClient(t)
	media := client.Medias[0]
	require.NoError(t, client.AddTrack(media, media.Codecs[0], core.NewReceiver(media, media.Codecs[0])))
	client.state = StatePlay
	require.NoError(t, client.Teardown())
	// Consume the response while deliberately keeping the TCP connection open.
	_, err := client.ReadResponse()
	require.NoError(t, err)
	require.Empty(t, client.setupChannels, "TEARDOWN invalidates every cached transport")
	require.NoError(t, client.AddTrack(media, media.Codecs[0], core.NewReceiver(media, media.Codecs[0])))
	require.Equal(t, int32(2), sessions.Load(), "a terminated playing session must reconnect")
	require.GreaterOrEqual(t, setups.Load(), int32(2), "the new session must negotiate SETUP")
	require.Equal(t, StateSetup, client.state)
	require.ErrorContains(t, client.Start(), "EOF")
	require.Equal(t, int32(1), plays.Load())
}

func newBackchannelLifecycleClient(t *testing.T) (*Conn, *atomic.Int32, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	var sessions, setups, plays atomic.Int32
	codec := &core.Codec{Name: core.CodecPCMU, ClockRate: 8000, PayloadType: 0}
	sdp, err := core.MarshalSDP("synthetic", []*core.Media{
		{Kind: core.KindAudio, Direction: core.DirectionSendonly, ID: "mic", Codecs: []*core.Codec{codec}},
		{Kind: core.KindAudio, Direction: core.DirectionSendonly, ID: "rejected", Codecs: []*core.Codec{codec}},
	})
	require.NoError(t, err)
	go func() {
		for {
			peer, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			session := sessions.Add(1)
			go func() {
				defer peer.Close()
				_ = peer.SetDeadline(time.Now().Add(5 * time.Second))
				reader := bufio.NewReader(peer)
				for {
					req, readErr := tcp.ReadRequest(reader)
					if readErr != nil {
						return
					}
					res := &tcp.Response{Proto: ProtoRTSP, Status: "200 OK", Header: map[string][]string{"CSeq": {req.Header.Get("CSeq")}, "Session": {fmt.Sprint(session)}}}
					switch req.Method {
					case MethodDescribe:
						res.Body = sdp
						res.Header.Set("Content-Type", "application/sdp")
						res.Header.Set("Content-Length", fmt.Sprint(len(sdp)))
					case MethodSetup:
						setups.Add(1)
						if strings.HasSuffix(req.URL.Path, "/rejected") {
							res.Status = "461 Unsupported Transport"
						} else {
							res.Header.Set("Transport", "RTP/AVP/TCP;unicast;interleaved=10-11")
						}
					case MethodPlay:
						plays.Add(1)
					}
					if res.Write(peer) != nil || req.Method == MethodPlay {
						return
					}
				}
			}()
		}
	}()
	client := NewClient("rtsp://" + listener.Addr().String() + "/stream")
	require.NoError(t, client.Dial())
	t.Cleanup(func() { _ = client.Stop() })
	require.NoError(t, client.Describe())
	return client, &sessions, &setups, &plays
}
