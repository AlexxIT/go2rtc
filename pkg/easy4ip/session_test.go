package easy4ip

import (
	"bytes"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeDevice is the device end of an established PTCP session: it acks, binds realms, and echoes
// payloads upper-cased. dropIn/dropOut lose that many data packets per direction to exercise recovery.
type fakeDevice struct {
	conn *net.UDPConn

	mu      sync.Mutex
	st      *ptcp
	client  atomic.Pointer[net.UDPAddr]
	closed  atomic.Bool // saw the 0x14 close command
	dropIn  atomic.Int32
	dropOut atomic.Int32
	mute    atomic.Bool // stop answering: a dead device
	bindsMu sync.Mutex
	binds   []uint32
}

func newFakeDevice(t *testing.T) *fakeDevice {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	d := &fakeDevice{conn: c, st: newPTCP()}
	go d.read()
	go d.retransmit()
	return d
}

func (d *fakeDevice) send(b *body) {
	d.mu.Lock()
	p := d.st.send(b)
	d.mu.Unlock()
	d.write(p)
}

func (d *fakeDevice) write(p *packet) {
	client := d.client.Load()
	if d.mute.Load() || client == nil {
		return
	}
	if p.body.size() > 0 && d.dropOut.Load() > 0 && d.dropOut.Add(-1) >= 0 {
		return // lost on the way; our own retransmit timer recovers it
	}
	_, _ = d.conn.WriteToUDP(p.marshal(), client)
}

func (d *fakeDevice) retransmit() {
	for range time.Tick(100 * time.Millisecond) {
		d.mu.Lock()
		due := d.st.dueRetransmits()
		d.mu.Unlock()
		for _, p := range due {
			d.write(p)
		}
	}
}

func (d *fakeDevice) read() {
	buf := make([]byte, 65536)
	for {
		n, from, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		d.client.Store(from)
		p, ok := parsePacket(append([]byte(nil), buf[:n]...))
		if !ok || d.mute.Load() {
			continue
		}
		if p.body.size() > 0 && p.body.kind == kindPayload && d.dropIn.Load() > 0 && d.dropIn.Add(-1) >= 0 {
			continue
		}

		hasBody := p.body.size() > 0
		d.mu.Lock()
		bodies := d.st.accept(p)
		var ack *packet
		if hasBody {
			ack = d.st.send(&body{kind: kindEmpty})
		}
		d.mu.Unlock()
		if ack != nil {
			d.write(ack)
		}

		for _, b := range bodies {
			switch {
			case b.kind == kindBind:
				d.bindsMu.Lock()
				d.binds = append(d.binds, b.port)
				d.bindsMu.Unlock()
				d.send(&body{kind: kindStatus, realm: b.realm, status: "CONN"})
			case b.kind == kindPayload:
				d.send(&body{kind: kindPayload, realm: b.realm, data: bytes.ToUpper(b.data)})
			case b.kind == kindStatus && b.status == "DISC":
				// the client closed a stream
			case b.kind == kindCommand && b.cmd[0] == 0x14:
				d.closed.Store(true)
			}
		}
	}
}

// startLive connects a live session to the fake device and runs it.
func startLive(t *testing.T, d *fakeDevice) *live {
	sock, err := listen()
	require.NoError(t, err)
	require.NoError(t, sock.connect(d.conn.LocalAddr().String()))
	l := newLive(sock, newPTCP())
	go l.serve()
	t.Cleanup(l.shutdown)
	return l
}

func readN(t *testing.T, s *Stream, n int) string {
	got := make([]byte, 0, n)
	buf := make([]byte, 1024)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for len(got) < n {
			m, err := s.Read(buf)
			if err != nil {
				return
			}
			got = append(got, buf[:m]...)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out after %d of %d bytes", len(got), n)
	}
	return string(got)
}

func TestStreamEcho(t *testing.T) {
	d := newFakeDevice(t)
	l := startLive(t, d)

	s, err := l.open(554)
	require.NoError(t, err)
	d.bindsMu.Lock()
	require.Equal(t, []uint32{554}, d.binds)
	d.bindsMu.Unlock()

	_, err = s.Write([]byte("describe"))
	require.NoError(t, err)
	require.Equal(t, "DESCRIBE", readN(t, s, 8))

	// a write bigger than one payload is split, and comes back in order
	big := strings.Repeat("abcdefghij", 1500)
	_, err = s.Write([]byte(big))
	require.NoError(t, err)
	require.Equal(t, strings.ToUpper(big), readN(t, s, len(big)))
}

func TestStreamRecoversFromLoss(t *testing.T) {
	d := newFakeDevice(t)
	l := startLive(t, d)
	s, err := l.open(554)
	require.NoError(t, err)

	d.dropIn.Store(2)  // the device misses two of our packets: we retransmit after the RTO
	d.dropOut.Store(2) // we miss two of its packets: it retransmits
	payload := strings.Repeat("x", 3*chunk)
	_, err = s.Write([]byte(payload))
	require.NoError(t, err)
	require.Equal(t, strings.ToUpper(payload), readN(t, s, len(payload)), "no gap, no duplicate, in order")
}

func TestDeviceClosesStream(t *testing.T) {
	d := newFakeDevice(t)
	l := startLive(t, d)
	s, err := l.open(80)
	require.NoError(t, err)

	var realm uint32
	l.streamsMu.Lock()
	for r := range l.streams {
		realm = r
	}
	l.streamsMu.Unlock()
	d.send(&body{kind: kindStatus, realm: realm, status: "DISC"})

	_, err = s.Read(make([]byte, 8))
	require.ErrorIs(t, err, io.EOF)
}

func TestOpenRefused(t *testing.T) {
	old := bindTimeout
	bindTimeout = 300 * time.Millisecond
	t.Cleanup(func() { bindTimeout = old })

	d := newFakeDevice(t)
	d.mute.Store(true) // never answers the bind
	l := startLive(t, d)

	done := make(chan error, 1)
	go func() { _, err := l.open(554); done <- err }()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(bindTimeout + 3*time.Second):
		t.Fatal("open did not time out")
	}
	l.streamsMu.Lock()
	require.Empty(t, l.streams, "the failed stream is forgotten")
	l.streamsMu.Unlock()
}

func TestSilentDeviceEndsSession(t *testing.T) {
	old := deadAfter
	deadAfter = 500 * time.Millisecond
	t.Cleanup(func() { deadAfter = old })

	d := newFakeDevice(t)
	sock, err := listen()
	require.NoError(t, err)
	require.NoError(t, sock.connect(d.conn.LocalAddr().String()))
	l := newLive(sock, newPTCP())
	d.mute.Store(true)

	served := make(chan struct{})
	go func() { l.serve(); close(served) }()
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return for a silent device")
	}
	l.shutdown()
}

func TestSessionLostEndsStreams(t *testing.T) {
	d := newFakeDevice(t)
	l := startLive(t, d)
	s, err := l.open(554)
	require.NoError(t, err)

	l.shutdown()
	_, err = s.Read(make([]byte, 8))
	require.ErrorIs(t, err, io.EOF, "a lost session reads as EOF so the player reconnects")
	_, err = s.Write([]byte("x"))
	require.Error(t, err)
}

func TestCloseSendsCloseCommand(t *testing.T) {
	d := newFakeDevice(t)
	l := startLive(t, d)
	c := &Client{cfg: Config{}, changed: make(chan struct{}), stop: make(chan struct{}), cur: l}

	require.NoError(t, c.Close())
	require.Eventually(t, d.closed.Load, 2*time.Second, 10*time.Millisecond, "device got 0x14")
	_, err := c.Dial(554)
	require.ErrorIs(t, err, ErrClosed)
}

func TestCountsOffsetWraps(t *testing.T) {
	d := newFakeDevice(t)
	l := startLive(t, d)

	// start both sides just short of 2^32 so a few echoes cross it
	start := uint32(1<<32 - 3000)
	l.mu.Lock()
	l.st.sent, l.st.recv, l.lastSent, l.lastRecv = start, start, start, start
	l.mu.Unlock()
	d.mu.Lock()
	d.st.sent, d.st.recv = start, start
	d.mu.Unlock()

	s, err := l.open(554)
	require.NoError(t, err)
	payload := strings.Repeat("y", 4000)
	_, err = s.Write([]byte(payload))
	require.NoError(t, err)
	require.Equal(t, strings.ToUpper(payload), readN(t, s, len(payload)), "the data survives the wrap")

	l.mu.Lock()
	defer l.mu.Unlock()
	require.Equal(t, 1, l.recvWraps)
	require.Equal(t, 1, l.sentWraps)
}
