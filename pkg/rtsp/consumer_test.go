package rtsp

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestPartialWriteClosesConn(t *testing.T) {
	defer func(d time.Duration) { Timeout = d }(Timeout)
	Timeout = 50 * time.Millisecond

	server, client := net.Pipe()
	defer client.Close()

	c := &Conn{conn: server, state: StatePlay, playOK: true}
	codec := &core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96}
	handler := c.packetWriter(codec, 0, 96)
	packet := &rtp.Packet{Header: rtp.Header{Marker: true}, Payload: make([]byte, 20)}

	sent := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler(packet) // 36 bytes, the client reads only 7 of them, then the write times out
		close(sent)
		handler(packet)
		_ = c.WriteResponse(&tcp.Response{})
	}()

	b := make([]byte, 7)
	_, err := io.ReadFull(client, b)
	require.NoError(t, err)
	require.Equal(t, []byte{'$', 0, 0, 32}, b[:4])

	<-sent

	// nothing may follow the incomplete message: the peer would read it as the missing tail
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	n, err := client.Read(make([]byte, 64))
	require.Equal(t, 0, n)
	require.ErrorIs(t, err, io.EOF)
	<-done
}
