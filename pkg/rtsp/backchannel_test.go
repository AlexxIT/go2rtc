package rtsp

import (
	"bufio"
	"io"
	"net"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/probe"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestBackchannelReusesSetupForNewSource(t *testing.T) {
	oldTimeout := Timeout
	Timeout = time.Second
	defer func() { Timeout = oldTimeout }()
	conn, peer := net.Pipe()
	defer conn.Close()
	defer peer.Close()
	require.NoError(t, peer.SetDeadline(time.Now().Add(5*time.Second)))
	codec := &core.Codec{Name: core.CodecPCMU, ClockRate: 8000, PayloadType: 0}
	media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: []*core.Codec{codec}, ID: "backchannel"}
	videoCodec := &core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96}
	videoMedia := &core.Media{Kind: core.KindVideo, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{videoCodec}, ID: "video"}
	client := NewClient("rtsp://127.0.0.1:1/stream")
	client.URL, _ = url.Parse(client.uri)
	client.conn, client.reader, client.auth = conn, bufio.NewReader(conn), tcp.NewAuth(nil)
	client.mode, client.state, client.Medias = core.ModeActiveProducer, StateConn, []*core.Media{videoMedia, media}
	defer func() {
		for _, sender := range client.Senders {
			sender.Close()
		}
	}()
	var setups, teardowns atomic.Int32
	packets := make(chan []byte, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		reader := bufio.NewReader(peer)
		for {
			prefix, err := reader.Peek(1)
			if err != nil {
				return
			}
			if prefix[0] == '$' {
				header := make([]byte, 4)
				if _, err = io.ReadFull(reader, header); err != nil {
					return
				}
				packet := make([]byte, int(header[2])<<8|int(header[3]))
				if _, err = io.ReadFull(reader, packet); err != nil {
					return
				}
				packets <- append(header, packet...)
				continue
			}
			req, err := tcp.ReadRequest(reader)
			if err != nil {
				return
			}
			if req.Method == MethodTeardown {
				teardowns.Add(1)
				return
			}
			if req.Method == MethodSetup {
				setups.Add(1)
			}
			res := &tcp.Response{Proto: ProtoRTSP, Status: "200 OK", Header: map[string][]string{
				"CSeq": {req.Header.Get("CSeq")}, "Session": {"test-session"},
				"Transport": {"RTP/AVP/TCP;unicast;interleaved=10-11"},
			}}
			if req.URL.Path == "/stream/video" {
				res.Header.Set("Transport", "RTP/AVP/TCP;unicast;interleaved=0-1")
			}
			if err = res.Write(peer); err != nil {
				return
			}
			if req.Method == MethodPlay || req.Method == MethodOptions {
				packet := &rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 96}, Payload: []byte{0x61, 1}}
				payload, _ := packet.Marshal()
				if _, err = peer.Write(append([]byte{'$', 0, 0, byte(len(payload))}, payload...)); err != nil {
					return
				}
			}
		}
	}()
	video, err := client.GetTrack(videoMedia, videoCodec)
	require.NoError(t, err)
	firstProbe := probe.Create("test", url.Values{"microphone": {""}})
	first, err := firstProbe.GetTrack(firstProbe.Medias[0], codec)
	require.NoError(t, err)
	require.NoError(t, client.AddTrack(media, codec, first))
	require.NoError(t, firstProbe.Stop())
	client.Senders[0].Close()
	client.Senders[0].Wait()
	require.NoError(t, client.Play())
	require.NoError(t, client.handleTCPData())
	require.NoError(t, client.handleTCPData())
	require.Equal(t, 1, video.Packets)
	client.state = StatePlay
	secondProbe := probe.Create("test", url.Values{"microphone": {""}})
	defer secondProbe.Stop()
	second, err := secondProbe.GetTrack(secondProbe.Medias[0], codec)
	require.NoError(t, err)
	require.NoError(t, client.AddTrack(media, codec, second))
	require.Equal(t, StatePlay, client.state)
	require.Equal(t, int32(2), setups.Load())
	require.Zero(t, teardowns.Load())
	require.NoError(t, client.Options())
	require.NoError(t, client.handleTCPData())
	require.Equal(t, 2, video.Packets)
	second.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, Marker: true, SequenceNumber: 7}, Payload: []byte{1, 2, 3}})
	select {
	case data := <-packets:
		require.Equal(t, byte(10), data[1])
		packet := new(rtp.Packet)
		require.NoError(t, packet.Unmarshal(data[4:]))
		require.Equal(t, []byte{1, 2, 3}, packet.Payload)
	case <-time.After(time.Second):
		t.Fatal("new backchannel source did not send RTP")
	}
	// A different media still requires reconnecting an active session.
	other := media.Clone()
	other.ID = "other-backchannel"
	client.uri = "://invalid"
	require.Error(t, client.AddTrack(other, codec, core.NewReceiver(other, codec)))
	<-done
	require.Equal(t, int32(1), teardowns.Load())
	require.Empty(t, client.setupChannels)
}

func TestDialClearsBackchannelSetup(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	client := NewClient("rtsp://" + listener.Addr().String() + "/stream")
	client.setupChannels = map[*core.Media]byte{{}: 10}
	require.NoError(t, client.Dial())
	defer client.conn.Close()
	require.Empty(t, client.setupChannels)
	client.setupChannels = map[*core.Media]byte{{}: 10}
	client.uri = "://invalid"
	require.Error(t, client.Dial())
	require.Empty(t, client.setupChannels)
}
