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
	defer server.Close()
	defer client.Close()

	require.NoError(t, client.SetReadDeadline(time.Now().Add(time.Second)))

	c := &Conn{conn: server, state: StatePlay, playOK: true}
	codec := &core.Codec{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96}
	handler := c.packetWriter(codec, 0, 96)
	packet := &rtp.Packet{
		Header:  rtp.Header{Marker: true},
		Payload: make([]byte, 20),
	}

	sent := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)

		handler(packet)
		close(sent)

		handler(packet)
		_ = c.WriteResponse(&tcp.Response{})
	}()

	defer func() {
		_ = client.Close()
		_ = server.Close()
		<-done
	}()

	b := make([]byte, 7)
	_, err := io.ReadFull(client, b)
	require.NoError(t, err)
	require.Equal(t, []byte{'$', 0, 0, 32}, b[:4])

	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("write timeout")
	}

	// Nothing may follow an incomplete RTSP message.
	n, err := client.Read(make([]byte, 64))
	require.Zero(t, n)
	require.ErrorIs(t, err, io.EOF)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("writer did not stop")
	}
}
