package easy4ip

import (
	"bytes"
	"encoding/binary"
	"time"
)

// PTCP is Dahua's reliable stream over UDP: every packet carries a byte offset (sent), the peer's
// next expected offset (recv) and two packet ids. This is a port of khoanguyen-3fc/dh-p2p (MIT) with
// the fixes found against a real XVR: NACKs, re-stamped retransmits and wrapping u32 offsets.
// See LICENSE in this directory for the original copyright notice.

type bodyKind byte

const (
	kindEmpty bodyKind = iota
	kindSync
	kindCommand
	kindPayload
	kindBind
	kindStatus
	kindHeartbeat
)

// body is the part of a PTCP packet after the 24-byte header.
type body struct {
	kind   bodyKind
	cmd    []byte // kindCommand
	realm  uint32 // kindPayload, kindBind, kindStatus
	data   []byte // kindPayload
	port   uint32 // kindBind
	status string // kindStatus
}

func (b *body) size() int {
	switch b.kind {
	case kindSync:
		return 4
	case kindCommand:
		return len(b.cmd)
	case kindPayload:
		return len(b.data) + 12
	case kindBind:
		return 20
	case kindStatus:
		return len(b.status) + 12
	case kindHeartbeat:
		return 12
	}
	return 0
}

func (b *body) marshal() []byte {
	switch b.kind {
	case kindSync:
		return []byte{0x00, 0x03, 0x01, 0x00}
	case kindCommand:
		return b.cmd
	case kindPayload:
		out := make([]byte, 12, 12+len(b.data))
		binary.BigEndian.PutUint32(out, 0x10000000|uint32(len(b.data)))
		binary.BigEndian.PutUint32(out[4:], b.realm)
		return append(out, b.data...)
	case kindBind:
		out := make([]byte, 20)
		out[0] = 0x11
		binary.BigEndian.PutUint32(out[4:], b.realm)
		binary.BigEndian.PutUint32(out[12:], b.port)
		copy(out[16:], []byte{127, 0, 0, 1})
		return out
	case kindStatus:
		out := make([]byte, 12, 12+len(b.status))
		out[0] = 0x12
		binary.BigEndian.PutUint32(out[4:], b.realm)
		return append(out, b.status...)
	case kindHeartbeat:
		out := make([]byte, 12)
		out[0] = 0x13
		return out
	}
	return nil
}

func parseBody(data []byte) (*body, bool) {
	if len(data) == 0 {
		return &body{kind: kindEmpty}, true
	}
	if len(data) < 4 {
		return nil, false
	}
	switch data[0] {
	case 0x00:
		return &body{kind: kindSync}, true
	case 0x10:
		if len(data) < 12 {
			return nil, false
		}
		length := binary.BigEndian.Uint32(data) & 0xFFFF
		padding := binary.BigEndian.Uint32(data[8:])
		if padding != 0 || int(length) != len(data)-12 {
			return nil, false
		}
		return &body{kind: kindPayload, realm: binary.BigEndian.Uint32(data[4:]), data: data[12:]}, true
	case 0x11:
		if len(data) >= 16 {
			return &body{kind: kindBind, realm: binary.BigEndian.Uint32(data[4:]), port: binary.BigEndian.Uint32(data[12:])}, true
		}
	case 0x12:
		if len(data) >= 12 {
			return &body{kind: kindStatus, realm: binary.BigEndian.Uint32(data[4:]), status: string(data[12:])}, true
		}
	case 0x13:
		return &body{kind: kindHeartbeat}, true
	}
	return &body{kind: kindCommand, cmd: data}, true
}

type packet struct {
	sent, recv, pid, lmid, rmid uint32
	body                        *body
}

func parsePacket(data []byte) (*packet, bool) {
	if len(data) < 24 || !bytes.Equal(data[:4], []byte("PTCP")) {
		return nil, false
	}
	b, ok := parseBody(data[24:])
	if !ok {
		return nil, false
	}
	return &packet{
		sent: binary.BigEndian.Uint32(data[4:]),
		recv: binary.BigEndian.Uint32(data[8:]),
		pid:  binary.BigEndian.Uint32(data[12:]),
		lmid: binary.BigEndian.Uint32(data[16:]),
		rmid: binary.BigEndian.Uint32(data[20:]),
		body: b,
	}, true
}

func (p *packet) marshal() []byte {
	out := make([]byte, 24, 24+p.body.size())
	copy(out, "PTCP")
	binary.BigEndian.PutUint32(out[4:], p.sent)
	binary.BigEndian.PutUint32(out[8:], p.recv)
	binary.BigEndian.PutUint32(out[12:], p.pid)
	binary.BigEndian.PutUint32(out[16:], p.lmid)
	binary.BigEndian.PutUint32(out[20:], p.rmid)
	return append(out, p.body.marshal()...)
}

// seqDiff is a-b in the u32 byte-offset space, so comparisons survive the wraparound (~4.5 h of a
// main stream). Never compare two offsets with < or > directly.
func seqDiff(a, b uint32) int32 { return int32(a - b) }

// rto is a fixed retransmit timeout, no RTT estimate.
// ponytail: add SRTT/RTTVAR if relay paths with high jitter retransmit too eagerly.
const rto = 500 * time.Millisecond

// maxOOO caps buffered out-of-order packets, ~1.3 MB at 1280 B each.
const maxOOO = 1024

type unacked struct {
	sent, end uint32
	lastSent  time.Time
	body      *body
}

// nackOffset: a device NACK is a command `0a 00 08 <offset u32 LE> ...` asking us to resend from that
// byte offset. It takes no sequence space (the next data packet reuses the same offset).
func nackOffset(b *body) (uint32, bool) {
	if b.kind == kindCommand && len(b.cmd) >= 7 && b.cmd[0] == 0x0a {
		return binary.LittleEndian.Uint32(b.cmd[3:]), true
	}
	return 0, false
}

// ptcp is the sender/receiver state of one session. Not safe for concurrent use: callers lock.
type ptcp struct {
	sent, recv uint32
	count      uint32
	id         uint32
	rmid       uint32
	ooo        map[uint32]*packet
	unacked    []unacked
}

func newPTCP() *ptcp { return &ptcp{ooo: map[uint32]*packet{}} }

// send stamps a body for transmission; whatever occupies sequence space is kept until acked.
func (s *ptcp) send(b *body) *packet {
	sent := s.sent
	n := uint32(b.size())
	s.sent += n

	if n > 0 {
		s.unacked = append(s.unacked, unacked{sent: sent, end: sent + n, lastSent: time.Now(), body: b})
	}
	return s.stamp(sent, b)
}

// stamp puts fresh packet ids and the current ack around a body at byte offset sent. Retransmits are
// re-stamped too: the device drops a resend that carries the old packet ids, and the whole session stalls.
func (s *ptcp) stamp(sent uint32, b *body) *packet {
	pid := 0x0000FFFF - s.count
	if b.kind == kindSync {
		pid = 0x0002FFFF
	}
	p := &packet{sent: sent, recv: s.recv, pid: pid, lmid: s.id, rmid: s.rmid, body: b}

	s.id++
	if b.kind != kindSync && b.kind != kindEmpty {
		s.count++
	}
	return p
}

// recvSync is the handshake path: strictly request/response, no reordering.
func (s *ptcp) recvSync(p *packet) *packet {
	s.recv += uint32(p.body.size())
	s.rmid = p.lmid
	return p
}

// accept is the data path: process the peer's ack, then return the bodies that are now in order.
// Duplicates are dropped, gaps are buffered until the device resends the hole.
func (s *ptcp) accept(p *packet) []*body {
	s.rmid = p.lmid

	for len(s.unacked) > 0 && seqDiff(s.unacked[0].end, p.recv) <= 0 {
		s.unacked = s.unacked[1:]
	}

	if p.body.size() == 0 {
		return nil
	}

	if from, ok := nackOffset(p.body); ok {
		// make everything from the requested offset due now; the retransmitter picks it up
		due := time.Now().Add(-rto)
		for i := range s.unacked {
			if seqDiff(s.unacked[i].end, from) > 0 {
				s.unacked[i].lastSent = due
			}
		}
		return nil
	}

	d := seqDiff(p.sent, s.recv)
	if d < 0 {
		return nil
	}
	if d > 0 {
		if len(s.ooo) < maxOOO {
			s.ooo[p.sent] = p
		}
		return nil
	}

	var out []*body
	for next := p; next != nil; next = s.ooo[s.recv] {
		delete(s.ooo, s.recv)
		s.recv += uint32(next.body.size())
		out = append(out, next.body)
	}
	for k := range s.ooo {
		if seqDiff(k, s.recv) <= 0 {
			delete(s.ooo, k)
		}
	}
	return out
}

// clearUnacked: handshake packets are answered synchronously, never resend them on the data path.
func (s *ptcp) clearUnacked() { s.unacked = nil }

// dueRetransmits returns packets whose ack is overdue, re-stamped, and marks them as just sent.
func (s *ptcp) dueRetransmits() []*packet {
	now := time.Now()
	var out []*packet
	for i := range s.unacked {
		u := &s.unacked[i]
		if now.Sub(u.lastSent) >= rto {
			u.lastSent = now
			out = append(out, s.stamp(u.sent, u.body))
		}
	}
	return out
}
