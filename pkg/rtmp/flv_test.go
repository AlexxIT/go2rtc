package rtmp

import (
	"bytes"
	"io"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv"
	"github.com/stretchr/testify/require"
)

// A tag with an empty payload fills a buffer of exactly the 15-byte header.
// The bounds hint in encodeFLV used to read index 15 and panic on it.
func TestEncodeFLVEmptyPayload(t *testing.T) {
	b := make([]byte, 4+11)
	encodeFLV(b, 9, 0x01020304, nil)
	require.Equal(t, []byte{0, 0, 0, 0, 9, 0, 0, 0, 0x02, 0x03, 0x04, 0x01, 0, 0, 0}, b)
}

func TestEncodeFLVPayload(t *testing.T) {
	b := make([]byte, 4+11+2)
	encodeFLV(b, 8, 0, []byte{0xaa, 0xbb})
	require.Equal(t, []byte{0, 0, 0, 0, 8, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0xaa, 0xbb}, b)
}

// message builds one RTMP message in a single type 0 chunk.
func message(csid, msgType byte, ts uint32, payload []byte) []byte {
	size := len(payload)
	b := []byte{
		csid, byte(ts >> 16), byte(ts >> 8), byte(ts),
		byte(size >> 16), byte(size >> 8), byte(size), msgType, 1, 0, 0, 0,
	}
	return append(b, payload...)
}

func newConn(b []byte) *Conn {
	return &Conn{
		rd:           bytes.NewReader(b),
		chunks:       map[byte]*chunk{},
		rdPacketSize: 128,
		rdBuf:        []byte{'F', 'L', 'V', 1, 0, 0, 0, 0, 9},
	}
}

var avcSequenceHeader = []byte{
	0x17, 0x00, 0x00, 0x00, 0x00, 0x01, 0x42, 0x00, 0x1f, 0xff, 0xe1, 0x00, 0x0a,
	0x27, 0x42, 0x00, 0x1f, 0x8d, 0x68, 0x05, 0x00, 0x5b, 0xa1, 0x01, 0x00, 0x04,
	0x28, 0xce, 0x3c, 0x80,
}

// Empty audio, video and data messages before and after the codec headers
// must not panic in encodeFLV, probe or Start (#2061).
func TestEmptyMessages(t *testing.T) {
	var b []byte
	b = append(b, message(4, TypeAudio, 0, nil)...)
	b = append(b, message(6, TypeVideo, 0, nil)...)
	b = append(b, message(5, TypeData, 0, nil)...)
	b = append(b, message(6, TypeVideo, 0, avcSequenceHeader)...)
	b = append(b, message(4, TypeAudio, 0, []byte{0xaf, 0x00, 0x12, 0x10})...)
	b = append(b, message(4, TypeAudio, 40, nil)...)
	b = append(b, message(6, TypeVideo, 40, nil)...)
	b = append(b, message(6, TypeVideo, 100, []byte{0x27, 0x01, 0, 0, 0, 0, 0, 0, 2, 0x41, 0x9a})...)
	b = append(b, message(4, TypeAudio, 100, []byte{0xaf, 0x01, 0x21, 0x00})...)

	prod, err := flv.Open(newConn(b))
	require.NoError(t, err)
	require.Len(t, prod.Medias, 2)

	var packets int
	for _, media := range prod.Medias {
		track, err := prod.GetTrack(media, media.Codecs[0])
		require.NoError(t, err)
		track.Input = func(*core.Packet) { packets++ }
	}

	require.Equal(t, io.EOF, prod.Start())
	require.Equal(t, 2, packets)
}

// A stream of only empty messages ends with EOF.
func TestOnlyEmptyMessages(t *testing.T) {
	var b []byte
	for i := 0; i < 100; i++ {
		b = append(b, message(4, TypeAudio, uint32(i), nil)...)
	}
	_, err := flv.Open(newConn(b))
	require.Equal(t, io.EOF, err)
}
