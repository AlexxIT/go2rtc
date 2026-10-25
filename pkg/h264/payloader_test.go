package h264

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmitNalusAVCCOverflow(t *testing.T) {
	// length prefix of 0xFFFFFFFE overflows when 4 is added to it, wrapping
	// the bounds check and reaching emit(nals[4:end]) with end < 4
	nals := make([]byte, 8)
	binary.BigEndian.PutUint32(nals, 0xFFFFFFFE)

	require.NotPanics(t, func() {
		EmitNalus(nals, true, func(b []byte) {
			t.Fatalf("unexpected emit for corrupt AVCC length: %x", b)
		})
	})
}
