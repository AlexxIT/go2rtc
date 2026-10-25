package h264

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmitNalusAVCC(t *testing.T) {
	// two NALs with valid 4-byte big-endian length prefixes
	nals := []byte{
		0, 0, 0, 2, 0xAA, 0xBB,
		0, 0, 0, 3, 0xCC, 0xDD, 0xEE,
	}

	var got [][]byte
	EmitNalus(nals, true, func(nalu []byte) {
		got = append(got, nalu)
	})

	require.Equal(t, [][]byte{{0xAA, 0xBB}, {0xCC, 0xDD, 0xEE}}, got)
}

func TestEmitNalusAVCCOverflow(t *testing.T) {
	// length prefix of 0xFFFFFFFE: 4+size overflows uint32 and wraps to 2,
	// which would pass the old `n < end` guard and panic on nals[4:end]
	nals := []byte{0xFF, 0xFF, 0xFF, 0xFE, 0x01}

	require.NotPanics(t, func() {
		EmitNalus(nals, true, func(nalu []byte) {
			t.Fatalf("unexpected emit for malformed AVCC data: %v", nalu)
		})
	})
}
