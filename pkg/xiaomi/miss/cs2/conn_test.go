package cs2

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testTimeout = 2 * time.Second

func newTestConn(t *testing.T, isTCP bool) (*Conn, net.Conn) {
	t.Helper()

	workerEnd, testEnd := net.Pipe()

	c := &Conn{
		Conn:  workerEnd,
		isTCP: isTCP,
		channels: [4]*dataChannel{
			newDataChannel(0, 10), nil, newDataChannel(250, 100), nil,
		},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.worker()
	}()

	t.Cleanup(func() {
		_ = testEnd.Close()
		_ = workerEnd.Close()

		select {
		case <-done:
		case <-time.After(testTimeout):
			t.Error("worker didn't stop")
		}
	})

	return c, testEnd
}

func writeConn(t *testing.T, conn net.Conn, b []byte) {
	t.Helper()

	require.NoError(t, conn.SetDeadline(time.Now().Add(testTimeout)))
	_, err := conn.Write(b)
	require.NoError(t, err)
}

func readConn(t *testing.T, conn net.Conn, n int) []byte {
	t.Helper()

	require.NoError(t, conn.SetDeadline(time.Now().Add(testTimeout)))
	buf := make([]byte, n)
	_, err := io.ReadFull(conn, buf)
	require.NoError(t, err)
	return buf
}

func drwFrame(ch byte, seq uint16, payload []byte) []byte {
	buf := make([]byte, 8+len(payload))
	buf[0] = magic
	buf[1] = msgDrw
	buf[4] = magicDrw
	buf[5] = ch
	binary.BigEndian.PutUint16(buf[6:], seq)
	copy(buf[8:], payload)
	return buf
}

func TestWorkerUnknownChannelUDP(t *testing.T) {
	_, conn := newTestConn(t, false)

	// Channel 3 is nil in channels, channel 9 is out of its bounds.
	for i, ch := range []byte{3, 9} {
		writeConn(t, conn, drwFrame(ch, 1, []byte("junk")))

		if i == 0 {
			// First packet triggers the 1s keepalive PING.
			require.Equal(t, []byte{magic, msgPing, 0, 0}, readConn(t, conn, 4))
		}

		ack := readConn(t, conn, 10)
		require.Equal(t, byte(magic), ack[0])
		require.Equal(t, byte(msgDrwAck), ack[1])
		require.Equal(t, ch, ack[5])
	}

	// Worker must still be alive after the unknown channels.
	writeConn(t, conn, []byte{magic, msgPing, 0, 0})
	require.Equal(t, []byte{magic, msgPong, 0, 0}, readConn(t, conn, 4))
}

func TestWorkerUnknownChannelTCP(t *testing.T) {
	_, conn := newTestConn(t, true)

	writeConn(t, conn, drwFrame(3, 1, []byte("junk")))

	// TCP worker sends a keepalive PING before dropping the packet.
	require.Equal(t, []byte{magic, msgPing, 0, 0}, readConn(t, conn, 4))

	// No ACK must follow: the next message is the reply to our PING.
	writeConn(t, conn, []byte{magic, msgPing, 0, 0})
	require.Equal(t, []byte{magic, msgPong, 0, 0}, readConn(t, conn, 4))
}

func TestWorkerKnownChannelUDP(t *testing.T) {
	c, conn := newTestConn(t, false)

	payload := []byte("command-payload")
	body := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(body, uint32(len(payload)))
	copy(body[4:], payload)

	writeConn(t, conn, drwFrame(0, 0, body))

	require.Equal(t, []byte{magic, msgPing, 0, 0}, readConn(t, conn, 4))

	ack := readConn(t, conn, 10)
	require.Equal(t, byte(msgDrwAck), ack[1])
	require.Equal(t, byte(0), ack[5])

	got, ok := c.channels[0].Pop()
	require.True(t, ok)
	require.Equal(t, payload, got)
}
