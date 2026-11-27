package legacy

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func entry(start uint32, dur, typ byte) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b, start)
	b[4], b[7] = dur, typ
	return b
}

func TestParseRecords(t *testing.T) {
	body := append(entry(1760000000, 60, 1), entry(1760000060, 30, 2)...)
	want := []Record{{1760000000, 60, 1}, {1760000060, 30, 2}}

	// first reply of a request carries a (kind, size) header in front
	header := make([]byte, 8)
	binary.LittleEndian.PutUint32(header, 1)
	binary.LittleEndian.PutUint32(header[4:], uint32(len(body)))

	require.Equal(t, want, parseRecords(append(header, body...)))
	require.Equal(t, want, parseRecords(body)) // a start time is far above 100, so no header
	require.Equal(t, want, parseRecords(append(body, 1, 2, 3)), "a partial trailing entry is ignored")
	require.Empty(t, parseRecords(nil))
}
