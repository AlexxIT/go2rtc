package dtls

import (
	"encoding/binary"
	"errors"
	"sync"
	"time"
)

const (
	// Keep selective ACKs within one DTLS record and bound retained media.
	mediaWindow  = 512
	mediaTimeout = 2 * time.Second
)

var errMediaGap = errors.New("tutk: media retransmission window exhausted")

// reliableMedia orders the shared audio/video packet counter before handing
// fragments to FrameHandler. Retransmissions have new DTLS record numbers but
// retain their inner packet counter, with channels 4/6/8 replacing 3/5/7.
type reliableMedia struct {
	mu                                      sync.Mutex
	enabled                                 bool
	pending                                 map[uint16][]byte
	next, highest, ackBase, ioSeq, ackCount uint16
	started                                 bool
	gapSince                                time.Time
	statsSeq, statsCount                    uint16
}

func (r *reliableMedia) enable() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = true
	r.pending = make(map[uint16][]byte)
	r.highest, r.ackBase, r.ioSeq = 0xffff, 0xffff, 0xffff
}

func (r *reliableMedia) handle(data []byte, now time.Time) (packets [][]byte, stats []byte, handled bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled || len(data) < 8 {
		return
	}
	r.statsCount++
	// Extended media packets carry the same statistics request as 0x080a.
	// Echo it promptly: the sender uses this response to estimate retransmit RTT.
	if (data[0] == 0x0c || data[0] == 0x0a) && data[1]&8 != 0 && len(data) >= 16 {
		stats = r.statistics(data)
	}
	if data[0] == 0x0a {
		handled = true
		return
	}
	if data[0] != 0x0c {
		return
	}
	offset := 16 // outer header plus FEC header
	if data[1]&8 != 0 {
		offset += 8
	}
	if len(data) < offset+20 {
		handled = true
		return
	}
	inner := data[offset:]
	switch inner[0] {
	case 0:
		seq := binary.LittleEndian.Uint16(inner[2:])
		if r.ioSeq == 0xffff || int16(seq-r.ioSeq) > 0 {
			r.ioSeq = seq
		}
		return
	case 0x0c:
		// FEC parity is optional when selective retransmission repairs losses.
		handled = true
		return
	case 3, 4, 5, 6, 7, 8:
	default:
		return
	}
	handled = true
	if int(binary.LittleEndian.Uint16(inner[8:])) != len(inner)-20 {
		return
	}
	seq := binary.LittleEndian.Uint16(inner[2:])
	if int16(seq-r.next) < 0 {
		return
	} // already emitted retransmission
	if uint16(seq-r.next) >= mediaWindow {
		err = errMediaGap
		return
	}
	if _, exists := r.pending[seq]; exists {
		return
	}
	packet := make([]byte, 8+len(inner))
	copy(packet, data[:8])
	copy(packet[8:], inner)
	packet[0], packet[1] = inner[0], inner[1]
	if packet[0]&1 == 0 {
		packet[0]--
		packet[8]--
	}
	r.pending[seq] = packet
	if !r.started || int16(seq-r.highest) > 0 {
		r.highest = seq
		r.started = true
	}
	previous := r.next
	for {
		packet, exists := r.pending[r.next]
		if !exists {
			break
		}
		packets = append(packets, packet)
		delete(r.pending, r.next)
		r.next++
	}
	if len(r.pending) == 0 {
		r.gapSince = time.Time{}
	} else if r.gapSince.IsZero() || previous != r.next {
		r.gapSince = now
	}
	return
}

// ack lists missing packet counters as offsets from ackBase. Advancing the
// base past an unresolved gap would let the sender discard the missing data.
func (r *reliableMedia) ack(now time.Time) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled {
		return nil, nil
	}
	if !r.gapSince.IsZero() && now.Sub(r.gapSince) >= mediaTimeout {
		return nil, errMediaGap
	}
	var missing []uint16
	if r.started {
		for seq := r.next; seq != r.highest+1; seq++ {
			if _, exists := r.pending[seq]; !exists {
				missing = append(missing, seq-r.ackBase)
			}
		}
	}
	b := make([]byte, max(24, 22+2*len(missing)))
	b[0] = 9
	binary.LittleEndian.PutUint16(b[2:], protoVersion)
	binary.LittleEndian.PutUint16(b[8:], r.ackBase)
	binary.LittleEndian.PutUint16(b[10:], r.highest)
	binary.LittleEndian.PutUint16(b[12:], r.ioSeq)
	binary.LittleEndian.PutUint16(b[14:], uint16(len(missing)))
	binary.LittleEndian.PutUint16(b[18:], r.ackCount)
	binary.LittleEndian.PutUint16(b[20:], uint16(now.UnixMilli()))
	for i, seq := range missing {
		binary.LittleEndian.PutUint16(b[22+2*i:], seq)
	}
	r.ackCount++
	r.ackBase = r.next - 1
	return b, nil
}

func (r *reliableMedia) statistics(data []byte) []byte {
	seq := binary.LittleEndian.Uint16(data[4:])
	expected := seq - r.statsSeq
	b := make([]byte, 20)
	b[0] = 11
	binary.LittleEndian.PutUint16(b[2:], protoVersion)
	copy(b[8:10], data[8:10])
	binary.LittleEndian.PutUint16(b[10:], expected)
	binary.LittleEndian.PutUint16(b[12:], r.statsCount)
	if expected > r.statsCount {
		binary.LittleEndian.PutUint16(b[14:], uint16(uint32(expected-r.statsCount)*100/uint32(expected)))
	}
	r.statsSeq = seq
	r.statsCount = 0
	return b
}
