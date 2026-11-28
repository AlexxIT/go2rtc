package rtmp

import (
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloseSendsUnpublish(t *testing.T) {
	client, server := net.Pipe()
	c := &Conn{conn: client, wr: client, wrPacketSize: 4096, streamID: 1, Stream: "streamkey", publishing: true}

	got := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(server)
		got <- b
	}()

	require.NoError(t, c.Close())

	b := string(<-got)
	require.Contains(t, b, "FCUnpublish")
	require.Contains(t, b, "deleteStream")
	require.Contains(t, b, "streamkey")
}

// A non-publishing connection (player or server side) closes without sending anything.
func TestCloseNoUnpublishWhenNotPublishing(t *testing.T) {
	client, server := net.Pipe()
	c := &Conn{conn: client, wr: client}

	got := make(chan int, 1)
	go func() {
		b, _ := io.ReadAll(server)
		got <- len(b)
	}()

	require.NoError(t, c.Close())
	require.Zero(t, <-got)
}
