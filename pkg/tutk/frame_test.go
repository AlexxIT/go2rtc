package tutk

import (
	"encoding/binary"
	"testing"
)

// The packet builders below mirror the layouts decoded in ParsePacketHeader
// and extractPayload for the video channel.

// videoStart builds a 36-byte-header start packet (no FrameInfo).
func videoStart(frameNo uint32, pktTotal, pktIdx uint16, payload []byte) []byte {
	b := make([]byte, 36+len(payload))
	b[0] = ChannelIVideo
	b[1] = FrameTypeStart
	binary.LittleEndian.PutUint16(b[20:], pktTotal)
	binary.LittleEndian.PutUint16(b[22:], pktIdx)
	binary.LittleEndian.PutUint16(b[24:], uint16(len(payload)))
	binary.LittleEndian.PutUint32(b[32:], frameNo)
	copy(b[36:], payload)
	return b
}

// videoCont builds a 28-byte-header continuation packet (no FrameInfo).
func videoCont(frameNo uint32, pktTotal, pktIdx uint16, payload []byte) []byte {
	b := make([]byte, 28+len(payload))
	b[0] = ChannelIVideo
	b[1] = FrameTypeCont
	binary.LittleEndian.PutUint16(b[12:], pktTotal)
	binary.LittleEndian.PutUint16(b[14:], pktIdx)
	binary.LittleEndian.PutUint16(b[16:], uint16(len(payload)))
	binary.LittleEndian.PutUint32(b[24:], frameNo)
	copy(b[28:], payload)
	return b
}

// videoEnd builds a 28-byte-header multi-packet end packet carrying the
// 40-byte FrameInfo. The 0x0028 marker makes ParsePacketHeader derive
// PktIdx as pktTotal-1 and flag HasFrameInfo.
func videoEnd(frameNo uint32, pktTotal uint16, payload []byte, codec byte, totalPayloadSize uint32) []byte {
	b := make([]byte, 28+len(payload)+frameInfoSize)
	b[0] = ChannelIVideo
	b[1] = FrameTypeEndMulti
	binary.LittleEndian.PutUint16(b[12:], pktTotal)
	binary.LittleEndian.PutUint16(b[14:], 0x0028) // marker -> HasFrameInfo
	binary.LittleEndian.PutUint16(b[16:], uint16(len(payload)))
	binary.LittleEndian.PutUint32(b[24:], frameNo)
	copy(b[28:], payload)

	fi := b[28+len(payload):]
	fi[0] = codec
	fi[2] = 0x01 // keyframe flag
	binary.LittleEndian.PutUint32(fi[16:], totalPayloadSize)
	binary.LittleEndian.PutUint32(fi[20:], frameNo)
	return b
}

// drain non-blockingly collects everything currently queued for output.
func drain(h *FrameHandler) []*Packet {
	var out []*Packet
	for {
		select {
		case p := <-h.output:
			out = append(out, p)
		default:
			return out
		}
	}
}

func TestHandleVideo_CleanFrame(t *testing.T) {
	h := NewFrameHandler(false)

	h.Handle(videoStart(1, 3, 0, []byte("AAAA")))
	h.Handle(videoCont(1, 3, 1, []byte("BBBB")))
	h.Handle(videoEnd(1, 3, []byte("CCCC"), CodecH264, 12))

	pkts := drain(h)
	if len(pkts) != 1 {
		t.Fatalf("expected 1 emitted packet, got %d", len(pkts))
	}
	if pkts[0].FrameNo != 1 {
		t.Errorf("FrameNo = %d, want 1", pkts[0].FrameNo)
	}
	if string(pkts[0].Payload) != "AAAABBBBCCCC" {
		t.Errorf("Payload = %q, want %q", pkts[0].Payload, "AAAABBBBCCCC")
	}
	if !pkts[0].IsKeyframe {
		t.Errorf("IsKeyframe = false, want true")
	}
	if h.OOODrops() != 0 {
		t.Errorf("OOODrops = %d, want 0", h.OOODrops())
	}
}

// TestHandleVideo_OutOfOrderDropAndRecover is the regression guard for #2215:
// an out-of-order packet must drop the frame exactly once (not once per
// remaining packet), and the next frame must still assemble cleanly.
func TestHandleVideo_OutOfOrderDropAndRecover(t *testing.T) {
	h := NewFrameHandler(false)

	// Frame #1, expecting packets 0,1,2,3 - but packet 2 is lost.
	h.Handle(videoStart(1, 4, 0, []byte("AAAA")))
	h.Handle(videoCont(1, 4, 1, []byte("BBBB")))
	h.Handle(videoCont(1, 4, 3, []byte("DDDD"))) // out of order -> drop frame

	if got := h.OOODrops(); got != 1 {
		t.Fatalf("after OOO packet: OOODrops = %d, want 1", got)
	}

	// A late/continuation packet of the already-dropped frame must be
	// absorbed silently and must NOT count as another drop.
	h.Handle(videoCont(1, 4, 2, []byte("CCCC")))
	if got := h.OOODrops(); got != 1 {
		t.Fatalf("after stray tail packet: OOODrops = %d, want 1 (no re-drop)", got)
	}

	if pkts := drain(h); len(pkts) != 0 {
		t.Fatalf("dropped frame should emit nothing, got %d packets", len(pkts))
	}

	// Recovery: a fresh frame after the drop must assemble normally.
	h.Handle(videoStart(2, 2, 0, []byte("EEEE")))
	h.Handle(videoEnd(2, 2, []byte("FFFF"), CodecH264, 8))

	pkts := drain(h)
	if len(pkts) != 1 {
		t.Fatalf("expected 1 recovered packet, got %d", len(pkts))
	}
	if pkts[0].FrameNo != 2 {
		t.Errorf("recovered FrameNo = %d, want 2", pkts[0].FrameNo)
	}
	if string(pkts[0].Payload) != "EEEEFFFF" {
		t.Errorf("recovered Payload = %q, want %q", pkts[0].Payload, "EEEEFFFF")
	}
	if got := h.OOODrops(); got != 1 {
		t.Errorf("final OOODrops = %d, want 1", got)
	}
}
