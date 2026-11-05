package rtsp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestServerReusesSessionForTracks(t *testing.T) {
	peer, conn := net.Pipe()
	defer peer.Close()
	defer conn.Close()
	require.NoError(t, peer.SetDeadline(time.Now().Add(5*time.Second)))
	server := NewServer(conn)
	server.mode = core.ModePassiveConsumer
	codecs := []*core.Codec{
		{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96},
		{Name: core.CodecPCMA, ClockRate: 8000, PayloadType: 8},
	}
	for _, codec := range codecs {
		media := &core.Media{Kind: core.GetKind(codec.Name), Direction: core.DirectionRecvonly, Codecs: []*core.Codec{codec}}
		track := core.NewReceiver(media, codec)
		require.NoError(t, server.AddTrack(media, codec, track))
	}
	defer func() {
		for _, sender := range server.Senders {
			sender.Close()
		}
	}()
	done := make(chan error, 1)
	go func() { done <- server.Accept() }()
	reader := bufio.NewReader(peer)
	session := ""
	for i := 0; i < 2; i++ {
		_, err := fmt.Fprintf(peer, "SETUP rtsp://camera.test/stream/trackID=%d RTSP/1.0\r\nCSeq: %d\r\nTransport: RTP/AVP/TCP;unicast;interleaved=%d-%d\r\nSession: %s\r\n\r\n", i, i+1, 2*i, 2*i+1, session)
		require.NoError(t, err)
		response, err := tcp.ReadResponse(reader)
		require.NoError(t, err)
		require.Equal(t, 200, response.StatusCode)
		current := strings.Split(response.Header.Get("Session"), ";")[0]
		require.NotEmpty(t, current)
		if i == 0 {
			session = current
		} else {
			require.Equal(t, session, current)
		}
		require.Equal(t, fmt.Sprintf("RTP/AVP/TCP;unicast;interleaved=%d-%d", 2*i, 2*i+1), response.Header.Get("Transport"))
	}
	_, err := fmt.Fprintf(peer, "PLAY rtsp://camera.test/stream RTSP/1.0\r\nCSeq: 3\r\nSession: %s\r\n\r\n", session)
	require.NoError(t, err)
	response, err := tcp.ReadResponse(reader)
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
	require.NoError(t, <-done)
	for i, sender := range server.Senders {
		sent := make(chan struct{})
		go func() {
			sender.Handler(&rtp.Packet{Header: rtp.Header{Version: 2, Marker: true, SequenceNumber: 1}, Payload: []byte{1, 2, 3}})
			close(sent)
		}()
		header := make([]byte, 4)
		_, err = io.ReadFull(reader, header)
		require.NoError(t, err)
		require.Equal(t, byte('$'), header[0])
		require.Equal(t, byte(2*i), header[1])
		payload := make([]byte, int(header[2])<<8|int(header[3]))
		_, err = io.ReadFull(reader, payload)
		require.NoError(t, err)
		packet := new(rtp.Packet)
		require.NoError(t, packet.Unmarshal(payload))
		require.Equal(t, []byte{1, 2, 3}, packet.Payload)
		<-sent
	}
}
