package legacy

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tutk"
)

// Frame layout from the Aqara G2 (lumi.camera.gwagl01) report in issue #2620:
// 8 byte nonce, flag 1, two LE uint32 (offset, size), then Annex B data.
func g2Frame(flag byte, annexb ...byte) []byte {
	b := []byte{1, 2, 3, 4, 5, 6, 7, 8, flag}
	b = append(b, 0, 0, 0, 0)                 // offset
	b = append(b, byte(len(annexb)), 0, 0, 0) // size
	return append(b, annexb...)
}

func TestVideoCodecSkipsInvalidFrame(t *testing.T) {
	// a frame that is not Annex B gives an empty AVCC: used to panic
	// "index out of range [4] with length 0" in h264.NALUType
	garbage := []byte{0xff, 0xf1, 0x60, 0x40, 0x0f, 0xe2, 0x0c, 0x01}

	for _, codec := range []byte{tutk.CodecH264, tutk.CodecH265} {
		if c := videoCodec(codec, garbage); c != nil {
			t.Fatalf("codec %x: expected nil, got %v", codec, c)
		}
		if c := videoCodec(codec, nil); c != nil {
			t.Fatalf("codec %x: expected nil for empty frame, got %v", codec, c)
		}
	}
}

func TestVideoCodecH264(t *testing.T) {
	sps := []byte{0, 0, 0, 1, 0x67, 0x42, 0x00, 0x2a, 0xd9, 0x40, 0x78, 0x02, 0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20}
	c := videoCodec(tutk.CodecH264, sps)
	if c == nil || c.Name != core.CodecH264 {
		t.Fatalf("expected H264 codec, got %v", c)
	}
}

func TestDecodePayloadAqaraG2H264(t *testing.T) {
	key := make([]byte, 32)
	start := []byte{0, 0, 0, 1, 0x67, 0x42, 0x00, 0x2a}
	payload := g2Frame(1, start...)

	got, err := decodePayload(ModelAqaraG2, tutk.CodecH264, payload, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(start) {
		t.Fatalf("H264 must use the video decoder: got %x, want %x", got, start)
	}
}
