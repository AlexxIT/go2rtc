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

func TestBackchannelAfterSuccessfulReconnect(t *testing.T) {
	oldTimeout := Timeout
	Timeout = time.Second
	defer func() { Timeout = oldTimeout }()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	var sessions, setups, teardowns atomic.Int32
	sdp, err := core.MarshalSDP("synthetic", []*core.Media{
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly, ID: "video", Codecs: []*core.Codec{{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96}}},
		{Kind: core.KindAudio, Direction: core.DirectionSendonly, ID: "mic", Codecs: []*core.Codec{{Name: core.CodecPCMU, ClockRate: 8000, PayloadType: 0}}},
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
				_ = peer.SetDeadline(time.Now().Add(10 * time.Second))
				reader := bufio.NewReader(peer)
				for {
					req, readErr := tcp.ReadRequest(reader)
					if readErr != nil {
						return
					}
					if req.Method == MethodTeardown {
						teardowns.Add(1)
						return
					}
					res := &tcp.Response{Proto: ProtoRTSP, Status: "200 OK", Header: map[string][]string{"CSeq": {req.Header.Get("CSeq")}, "Session": {fmt.Sprint(session)}}}
					if req.Method == MethodDescribe {
						res.Body = sdp
						res.Header.Set("Content-Type", "application/sdp")
						res.Header.Set("Content-Length", fmt.Sprint(len(sdp)))
					}
					if req.Method == MethodSetup {
						setups.Add(1)
						if strings.HasSuffix(req.URL.Path, "/mic") {
							res.Header.Set("Transport", "RTP/AVP/TCP;unicast;interleaved=10-11")
						} else {
							res.Header.Set("Transport", "RTP/AVP/TCP;unicast;interleaved=0-1")
						}
					}
					if res.Write(peer) != nil {
						return
					}
				}
			}()
		}
	}()
	client := NewClient("rtsp://" + listener.Addr().String() + "/stream")
	require.NoError(t, client.Dial())
	defer client.Stop()
	require.NoError(t, client.Describe())
	video, mic := client.Medias[0], client.Medias[1]
	_, err = client.GetTrack(video, video.Codecs[0])
	require.NoError(t, err)
	require.NoError(t, client.Play())
	require.NoError(t, client.handleTCPData())
	client.state = StatePlay
	require.NoError(t, client.AddTrack(mic, mic.Codecs[0], core.NewReceiver(mic, mic.Codecs[0])))
	require.Equal(t, int32(2), sessions.Load())
	require.Same(t, mic, client.Medias[1], "Describe must preserve existing media identity on reconnect")
	require.Equal(t, byte(10), client.setupChannels[mic])
	require.NoError(t, client.Play())
	require.NoError(t, client.handleTCPData())
	client.state = StatePlay
	require.NoError(t, client.AddTrack(client.Medias[1], mic.Codecs[0], core.NewReceiver(mic, mic.Codecs[0])))
	require.Equal(t, int32(2), sessions.Load(), "second probe must not reconnect")
	require.Equal(t, int32(3), setups.Load(), "initial video plus restored video and microphone")
	require.Equal(t, int32(1), teardowns.Load())
	require.Equal(t, StatePlay, client.state)
}
