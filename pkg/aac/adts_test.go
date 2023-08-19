package aac

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestADTS(t *testing.T) {
	// FFmpeg MPEG-TS AAC (one packet)
	s := "fff15080021ffc210049900219002380fff15080021ffc212049900219002380" //...
	src, err := hex.DecodeString(s)
	require.Nil(t, err)

	codec := ADTSToCodec(src)
	require.Equal(t, uint32(44100), codec.ClockRate)
	require.Equal(t, uint16(2), codec.Channels)

	size := ReadADTSSize(src)
	require.Equal(t, uint16(16), size)

	dst := CodecToADTS(codec)
	WriteADTSSize(dst, size)

	require.Equal(t, src[:len(dst)], dst)
}
