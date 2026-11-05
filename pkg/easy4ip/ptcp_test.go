package easy4ip

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func data(sent uint32, b byte) *packet {
	d := make([]byte, 8)
	for i := range d {
		d[i] = b
	}
	return &packet{sent: sent, body: &body{kind: kindPayload, realm: 1, data: d}}
}

func first(bodies []*body) (out []byte) {
	for _, b := range bodies {
		out = append(out, b.data[0])
	}
	return
}

func TestReorderDropDupAndWrap(t *testing.T) {
	s := newPTCP()
	start := uint32(math.MaxUint32 - 25) // packets are 20 bytes: offsets wrap during the test
	s.recv = start
	off := func(i uint32) uint32 { return start + 20*i }

	require.Equal(t, []byte{0}, first(s.accept(data(off(0), 0))))
	require.Empty(t, s.accept(data(off(2), 2))) // gap: buffered
	require.Empty(t, s.accept(data(off(3), 3)))
	require.Empty(t, s.accept(data(off(0), 0))) // duplicate
	require.Equal(t, off(1), s.recv)            // ack stays at the hole
	require.Equal(t, []byte{1, 2, 3}, first(s.accept(data(off(1), 1))))
	require.Equal(t, off(4), s.recv)
	require.Empty(t, s.ooo)
}

func TestNackIsUnsequencedAndTriggersResend(t *testing.T) {
	s := newPTCP()
	s.send(&body{kind: kindHeartbeat}) // 0..12
	s.send(&body{kind: kindHeartbeat}) // 12..24
	require.Empty(t, s.dueRetransmits())

	nack := data(0, 0)
	nack.body = &body{kind: kindCommand, cmd: []byte{0x0a, 0, 8, 12, 0, 0, 0, 0, 0, 0x87, 1}}
	require.Empty(t, s.accept(nack))
	require.Zero(t, s.recv) // the NACK took no sequence space

	resent := s.dueRetransmits()
	require.Len(t, resent, 1)
	require.EqualValues(t, 12, resent[0].sent)
	require.EqualValues(t, 2, resent[0].lmid) // re-stamped with a fresh id, not the original 1
}

func TestAckClearsUnacked(t *testing.T) {
	s := newPTCP()
	s.send(&body{kind: kindHeartbeat}) // 0..12
	s.send(&body{kind: kindHeartbeat}) // 12..24
	ack := data(0, 0)
	ack.body = &body{kind: kindEmpty}
	ack.recv = 12
	s.accept(ack)
	require.Len(t, s.unacked, 1)
}

func TestAckClearsAcrossWrap(t *testing.T) {
	s := newPTCP()
	s.sent = math.MaxUint32 - 5
	s.send(&body{kind: kindHeartbeat}) // wraps: end = 6
	ack := data(0, 0)
	ack.body = &body{kind: kindEmpty}
	ack.recv = 5
	s.accept(ack)
	require.Len(t, s.unacked, 1, "an ack short of the wrapped end keeps the packet")
	ack.recv = 6
	s.accept(ack)
	require.Empty(t, s.unacked)
}

func TestPacketRoundTrip(t *testing.T) {
	for name, b := range map[string]*body{
		"sync":      {kind: kindSync},
		"heartbeat": {kind: kindHeartbeat},
		"empty":     {kind: kindEmpty},
		"command":   {kind: kindCommand, cmd: []byte{0x17, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		"payload":   {kind: kindPayload, realm: 0xdeadbeef, data: []byte("DESCRIBE rtsp://x RTSP/1.0\r\n\r\n")},
		"bind":      {kind: kindBind, realm: 7, port: 554},
		"status":    {kind: kindStatus, realm: 7, status: "DISC"},
	} {
		in := &packet{sent: 1, recv: 2, pid: 3, lmid: 4, rmid: 5, body: b}
		out, ok := parsePacket(in.marshal())
		require.True(t, ok, name)
		require.Equal(t, in, out, name)
		require.Equal(t, b.size(), len(b.marshal()), name)
	}
}

func TestWireFormat(t *testing.T) {
	// byte-for-byte what dh-p2p sends
	p := &packet{sent: 0, recv: 0, pid: 0x0002FFFF, lmid: 0, rmid: 0, body: &body{kind: kindSync}}
	require.Equal(t, "PTCP"+"\x00\x00\x00\x00"+"\x00\x00\x00\x00"+"\x00\x02\xff\xff"+"\x00\x00\x00\x00"+"\x00\x00\x00\x00"+"\x00\x03\x01\x00", string(p.marshal()))

	bind := &body{kind: kindBind, realm: 0x01020304, port: 554}
	require.Equal(t, "\x11\x00\x00\x00\x01\x02\x03\x04\x00\x00\x00\x00\x00\x00\x02\x2a\x7f\x00\x00\x01", string(bind.marshal()))

	pay := &body{kind: kindPayload, realm: 1, data: []byte("ab")}
	require.Equal(t, "\x10\x00\x00\x02\x00\x00\x00\x01\x00\x00\x00\x00ab", string(pay.marshal()))
}

func TestParseRejectsGarbage(t *testing.T) {
	for name, b := range map[string][]byte{
		"short":       []byte("PTCP"),
		"magic":       append([]byte("XXXX"), make([]byte, 20)...),
		"length lies": append(append([]byte("PTCP"), make([]byte, 20)...), 0x10, 0, 0, 9, 0, 0, 0, 1, 0, 0, 0, 0, 'a'),
		"padding":     append(append([]byte("PTCP"), make([]byte, 20)...), 0x10, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 'a'),
		"tiny body":   append(append([]byte("PTCP"), make([]byte, 20)...), 1, 2),
	} {
		_, ok := parsePacket(b)
		require.False(t, ok, name)
	}
}

func TestRetransmitUsesFreshIDs(t *testing.T) {
	s := newPTCP()
	p := s.send(&body{kind: kindHeartbeat})
	s.unacked[0].lastSent = s.unacked[0].lastSent.Add(-2 * rto)
	r := s.dueRetransmits()
	require.Len(t, r, 1)
	require.Equal(t, p.sent, r[0].sent, "same byte offset")
	require.NotEqual(t, p.lmid, r[0].lmid)
	require.NotEqual(t, p.pid, r[0].pid)
}
