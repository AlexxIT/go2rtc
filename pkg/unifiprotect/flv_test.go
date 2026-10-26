package unifiprotect

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/flv"
	"github.com/AlexxIT/go2rtc/pkg/flv/amf"
	"github.com/stretchr/testify/require"
)

var flvHeader = []byte{
	'F', 'L', 'V', 1, 5,
	0, 0, 0, 9,
	0, 0, 0, 0,
}

func TestReaderExtendedFLV(t *testing.T) {
	metadata := amf.EncodeItems("onMetaData", map[string]any{
		"streamName":     "f4c2d8a19e7340b6",
		"audioFrequency": 16000,
		"audioChannels":  1,
		"videoWidth":     2688,
		"videoHeight":    1512,
	})

	want := []*Tag{
		{Type: TagData, Timestamp: 0, Data: metadata},
		{Type: TagVideo, Timestamp: 33, Data: []byte{0x17, 0, 0, 0, 0, 1, 2, 3}},
		{Type: TagOpus, Timestamp: 40, Data: []byte{0xcf, 0, 3, 2}},
		{Type: TagOpus, Timestamp: 60, Data: append([]byte{0xb8}, bytes.Repeat([]byte{0x55}, 160)...)},
	}

	var wire []byte
	wire = append(wire, flvHeader...)
	for i, tag := range want {
		wire = append(wire, flv.EncodeTag(tag.Type, tag.Timestamp, tag.Data)...)
		if i != len(want)-1 {
			wire = append(wire, bytes.Repeat([]byte{byte(0x40 + i)}, []int{16, 432, 784}[i])...)
		}
	}

	rd := NewReader(&chunkReader{data: wire, size: 7})
	for _, expected := range want {
		actual, err := rd.ReadTag()
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}
	_, err := rd.ReadTag()
	require.ErrorIs(t, err, io.EOF)

	got, ok, err := ParseMetadata(want[0])
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, Metadata{
		StreamName:     "f4c2d8a19e7340b6",
		AudioFrequency: 16000,
		AudioChannels:  1,
		VideoWidth:     2688,
		VideoHeight:    1512,
	}, got)
}

func TestReaderHeaderless(t *testing.T) {
	want := []*Tag{
		{Type: TagVideo, Timestamp: 0xffffffff, Data: []byte{0x17, 0, 0, 0, 0}},
		{Type: TagAudio, Timestamp: 20, Data: []byte{0xaf, 0, 0x14, 8}},
	}

	wire := appendTag(nil, want[0])
	wire = append(wire, bytes.Repeat([]byte{0xa5}, 20)...)
	wire = appendTag(wire, want[1])

	rd := NewReader(bytes.NewReader(wire))
	for _, expected := range want {
		actual, err := rd.ReadTag()
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}
}

func TestReaderRejectsInvalidHeader(t *testing.T) {
	header := append([]byte(nil), flvHeader...)
	binary.BigEndian.PutUint32(header[9:], 1)
	_, err := NewReader(bytes.NewReader(header)).ReadTag()
	require.ErrorIs(t, err, ErrFraming)
}

func TestParseMetadataIgnoresOtherData(t *testing.T) {
	tag := &Tag{Type: TagData, Data: amf.EncodeItems("onClockSync", map[string]any{})}
	_, ok, err := ParseMetadata(tag)
	require.NoError(t, err)
	require.False(t, ok)
}

func appendTag(dst []byte, tag *Tag) []byte {
	return append(dst, flv.EncodeTag(tag.Type, tag.Timestamp, tag.Data)...)
}

type chunkReader struct {
	data []byte
	size int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := r.size
	if n > len(r.data) {
		n = len(r.data)
	}
	if n > len(p) {
		n = len(p)
	}
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}
