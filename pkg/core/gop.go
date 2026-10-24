package core

import "sync"

// gopMaxPackets bounds the cache: a stream that goes this long without a
// keyframe is not cached at all until the next one (about 6 s of a 1080p
// stream at 2-3 Mbit/s).
const gopMaxPackets = 2048

// gop holds the packets since the last keyframe, so a consumer that joins
// mid-stream can start decoding at once instead of waiting for the next
// keyframe (HomeKit live view waits up to a whole GOP otherwise).
type gop struct {
	mu         sync.Mutex
	isKeyframe func(*Packet) bool
	packets    []*Packet
}

// add appends the packet, restarting the cache at a keyframe that opens a new
// access unit (an SPS, PPS and IDR slice of one frame share a timestamp).
func (g *gop) add(packet *Packet) {
	if g.isKeyframe(packet) && (len(g.packets) == 0 || g.packets[0].Timestamp != packet.Timestamp) {
		g.packets = g.packets[:0]
		g.packets = append(g.packets, packet)
		return
	}
	if len(g.packets) == 0 {
		return // no keyframe seen yet
	}
	if len(g.packets) >= gopMaxPackets {
		g.packets = g.packets[:0]
		return
	}
	g.packets = append(g.packets, packet)
}

// KeepGOP makes the receiver keep its packets since the last keyframe for
// consumers joining with Sender.WithParentFromKeyframe. Idempotent.
func (r *Receiver) KeepGOP(isKeyframe func(*Packet) bool) {
	r.gop.CompareAndSwap(nil, &gop{isKeyframe: isKeyframe})
}

// WithParentFromKeyframe attaches the sender like WithParent, first handing it
// the receiver's packets since the last keyframe, if it keeps them. The replay
// and the attach happen under the cache's lock, which Receiver.Input also
// holds while it caches and dispatches a packet: no packet is lost or doubled.
func (s *Sender) WithParentFromKeyframe(parent *Receiver) *Sender {
	g := parent.gop.Load()
	if g == nil {
		return s.WithParent(parent)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	for _, packet := range g.packets {
		s.Input(packet)
	}
	return s.WithParent(parent)
}
