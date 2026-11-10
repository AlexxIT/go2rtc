package legacy

import (
	"errors"
	"testing"
	"time"

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

type fakeReader struct {
	packets [][2][]byte
	reads   int
}

func (f *fakeReader) SetDeadline(time.Time) error { return nil }

func (f *fakeReader) ReadPacket() (hdr, payload []byte, err error) {
	if f.reads >= len(f.packets) {
		return nil, nil, errors.New("timeout")
	}
	p := f.packets[f.reads]
	f.reads++
	return p[0], p[1], nil
}

func TestProbeVideoOnly(t *testing.T) {
	sps := []byte{0, 0, 0, 1, 0x67, 0x42, 0x00, 0x2a, 0xd9, 0x40, 0x78, 0x02, 0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20}
	frame := [2][]byte{{tutk.CodecH264}, {0, 0, 0, 1, 0x41, 0x01}}

	// a camera without audio: probe must stop by itself, not wait for the deadline
	f := &fakeReader{packets: [][2][]byte{{{tutk.CodecH264}, sps}}}
	for i := 0; i < 20; i++ {
		f.packets = append(f.packets, frame)
	}

	medias, err := probe(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(medias) != 1 || medias[0].Kind != core.KindVideo {
		t.Fatalf("expected one video media, got %v", medias)
	}
	if f.reads > probeAfterVideo+2 {
		t.Fatalf("probe read %d packets, expected it to stop soon after video", f.reads)
	}
}
