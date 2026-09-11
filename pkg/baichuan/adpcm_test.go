package baichuan

import (
	"testing"
)

func TestADPCMDecoder(t *testing.T) {
	// A small dummy DVI block with a 4-byte state header.
	data := []byte{0x00, 0x00, 0x00, 0x00, 0x12, 0x34, 0x56, 0x78}
	decoder := &ADPCMDecoder{}
	pcm := decoder.Decode(data)

	if len(pcm) != (len(data)-4)*2 {
		t.Fatalf("expected pcm length %d, got %d", (len(data)-4)*2, len(pcm))
	}

	// Just verify it doesn't panic and state is updated
	if decoder.index == 0 && decoder.predicted == 0 {
		t.Errorf("decoder state should have updated after decoding")
	}
}
