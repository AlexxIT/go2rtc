package tutk

import (
	"encoding/binary"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// sink is a net.Conn that remembers what the session wrote.
type sink struct {
	net.Conn
	mu  sync.Mutex
	out [][]byte
}

func (s *sink) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.out = append(s.out, append([]byte(nil), b...))
	return len(b), nil
}

func (s *sink) types() (types []uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.out {
		types = append(types, binary.LittleEndian.Uint16(b[msgHhrSize+4:]))
	}
	return
}

// camFrame is a ch1 frame as the camera sends it (the part after the TUTK header).
func camFrame(typ uint16, seq, tag uint32, body []byte) []byte {
	b := make([]byte, 20+len(body))
	copy(b, ch1Magic)
	binary.LittleEndian.PutUint16(b[4:], typ)
	binary.LittleEndian.PutUint16(b[6:], uint16(len(body)))
	binary.LittleEndian.PutUint32(b[8:], seq)
	binary.LittleEndian.PutUint32(b[16:], tag)
	copy(b[20:], body)
	return b
}

func TestCh1Handshake(t *testing.T) {
	w := &sink{}
	s := NewSession16(w, make([]byte, 8))

	require.False(t, s.handleCh1RUDP([]byte("not a ch1 frame at all, just bytes")))

	// camera hello: we ack it, say hello ourselves once, and learn the camera's tag
	require.True(t, s.handleCh1RUDP(camFrame(0x0501, 0, 0x500, nil)))
	require.Equal(t, uint32(0x500), s.ch1.camTag)
	require.Equal(t, []uint16{0x0541, 0x0501}, w.types())

	// the second hello is only acked
	require.True(t, s.handleCh1RUDP(camFrame(0x0501, 0, 0x500, nil)))
	require.Equal(t, []uint16{0x0541, 0x0501, 0x0541}, w.types())

	// the camera acks our hello: the stream is open
	require.True(t, s.handleCh1RUDP(camFrame(0x0541, 0, 0x100, nil)))
	select {
	case <-s.ch1.ready:
	default:
		t.Fatal("ready not closed")
	}
}

func TestCh1Data(t *testing.T) {
	w := &sink{}
	s := NewSession16(w, make([]byte, 8))

	require.True(t, s.handleCh1RUDP(camFrame(0x0502, 7, 0x100, []byte("hello"))))

	require.Equal(t, []uint16{0x0546}, w.types(), "every data frame is acked")
	ack := w.out[0][msgHhrSize:]
	require.EqualValues(t, 7, binary.LittleEndian.Uint32(ack[8:]), "ack carries the frame's seq")
	require.Equal(t, []byte("hello"), <-s.ch1.data)
}

func TestCh1TruncatedData(t *testing.T) {
	s := NewSession16(&sink{}, make([]byte, 8))
	f := camFrame(0x0502, 0, 0x100, []byte("hello"))
	require.True(t, s.handleCh1RUDP(f[:len(f)-2])) // size says 5, only 3 bytes arrived
	require.Empty(t, s.ch1.data)
}
