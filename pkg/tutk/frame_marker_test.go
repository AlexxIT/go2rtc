package tutk

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func TestFrameInfoMarker(t *testing.T) {
	cases := []struct {
		ft       uint8
		v, total uint16
		want     int
	}{
		{FrameTypeEndMulti, 0x28, 5, 40}, // newer firmware
		{FrameTypeEndMulti, 48, 16, 48},  // Doorbell v1
		{FrameTypeStartAlt, 48, 2, 48},   // Doorbell v1 StartAlt carrying FI
		{FrameTypeEndMulti, 4, 5, 0},     // real last index
		{FrameTypeEndMulti, 48, 72, 0},   // 48 is a real index when total=72 (start of frame)
		{FrameTypeCont, 3, 16, 0},        // continuation
	}
	for _, c := range cases {
		if got := frameInfoMarker(c.ft, c.v, c.total); got != c.want {
			t.Errorf("frameInfoMarker(%#x,%d,%d)=%d want %d", c.ft, c.v, c.total, got, c.want)
		}
	}
}

// Packet captured from a WYZEDB3 on firmware 4.25.1.333: single-packet P-frame
// (ch 0x07, type 0x05) with a 48-byte FRAMEINFO.
func TestDoorbellV1SinglePacketFrame(t *testing.T) {
	hdr, _ := hex.DecodeString("07050b0064001d7707015a0001003000e603000004000000030000000000000121f61b09edff001d")
	fi, _ := hex.DecodeString("4e000000000f030001000000b9eae76ab6030000072d0400000000000000000000000000000000000000000000000000")
	payloadLen := int(binary.LittleEndian.Uint16(hdr[16:])) - len(fi) // 998
	pkt := make([]byte, 28+payloadLen+len(fi))
	copy(pkt, hdr[:28])
	copy(pkt[28:], hdr[28:])
	copy(pkt[28+payloadLen:], fi)

	h := NewFrameHandler(false)
	h.Handle(pkt)
	select {
	case p := <-h.Recv():
		if p.Codec != CodecH264 || len(p.Payload) != payloadLen {
			t.Fatalf("got codec=%#x len=%d", p.Codec, len(p.Payload))
		}
	default:
		t.Fatal("no frame emitted")
	}
}

func mkPkt28(ch, ft byte, total, idxOrMarker uint16, frameNo uint32, payload []byte) []byte {
	b := make([]byte, 28+len(payload))
	b[0], b[1] = ch, ft
	binary.LittleEndian.PutUint16(b[12:], total)
	binary.LittleEndian.PutUint16(b[14:], idxOrMarker)
	binary.LittleEndian.PutUint16(b[16:], uint16(len(payload)))
	binary.LittleEndian.PutUint32(b[24:], frameNo)
	copy(b[28:], payload)
	return b
}

// A keyframe with more packets than the marker value (seen on the doorbell
// as 72-packet keyframes ending with marker 48) must still assemble.
func TestLargeFrameEndingWithMarker(t *testing.T) {
	const total = 60
	h := NewFrameHandler(false)
	var want int
	for i := 0; i < total-1; i++ {
		p := make([]byte, 100)
		want += len(p)
		h.Handle(mkPkt28(ChannelIVideo, FrameTypeCont, total, uint16(i), 7, p))
	}
	last := make([]byte, 30)
	want += len(last)
	fi := make([]byte, 48)
	fi[0], fi[2] = CodecH264, 1
	binary.LittleEndian.PutUint32(fi[16:], uint32(want))
	h.Handle(mkPkt28(ChannelIVideo, FrameTypeEndMulti, total, 48, 7, append(last, fi...)))
	select {
	case p := <-h.Recv():
		if len(p.Payload) != want || !p.IsKeyframe {
			t.Fatalf("got len=%d key=%v want %d", len(p.Payload), p.IsKeyframe, want)
		}
	default:
		t.Fatal("no frame emitted")
	}
}
