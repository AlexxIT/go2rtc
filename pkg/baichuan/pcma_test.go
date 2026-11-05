package baichuan

import (
	"testing"
)

func TestEncodePCMA(t *testing.T) {
	pcm := []int16{0, 1000, 32767, -1000, -32768, 5, -5}
	pcma := EncodePCMA(pcm)

	if len(pcma) != len(pcm) {
		t.Fatalf("expected pcma length %d, got %d", len(pcm), len(pcma))
	}

	// Basic A-Law conversion sanity checks
	// 0 usually maps to 0xD5 (which is 0x55 ^ 0x80)
	if pcma[0] != 0xD5 {
		t.Errorf("expected 0 to encode to 0xD5, got %#x", pcma[0])
	}
}
