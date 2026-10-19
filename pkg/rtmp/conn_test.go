package rtmp

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteMessageExtendedTimestamp(t *testing.T) {
	var buf bytes.Buffer
	c := &Conn{wr: &buf, wrPacketSize: 4096}

	require.NoError(t, c.writeMessage(4, TypeVideo, 0xFFFFFF, []byte{0x01, 0x02}))

	got := buf.Bytes()
	// basic header(1) + message header(11) + extended timestamp(4) + payload(2)
	require.Len(t, got, 1+11+4+2)
	require.Equal(t, byte(4), got[0])                    // fmt 0, chunk id 4
	require.Equal(t, []byte{0xFF, 0xFF, 0xFF}, got[1:4]) // sentinel
	require.Equal(t, []byte{0x00, 0x00, 0x02}, got[4:7]) // message length
	require.Equal(t, byte(TypeVideo), got[7])
	require.Equal(t, uint32(0xFFFFFF), binary.BigEndian.Uint32(got[12:16])) // extended timestamp
	require.Equal(t, []byte{0x01, 0x02}, got[16:])
}

func TestWriteMessageExtendedTimestampChunked(t *testing.T) {
	var buf bytes.Buffer
	c := &Conn{wr: &buf, wrPacketSize: 4} // tiny chunk size to force a continuation

	require.NoError(t, c.writeMessage(4, TypeVideo, 0x1000000, []byte{1, 2, 3, 4, 5, 6}))

	got := buf.Bytes()
	// type 0: basic(1) + header(11) + ext(4) + 4 payload = 20
	// type 3: basic(1) + ext(4) + 2 payload = 7
	require.Len(t, got, 20+7)

	require.Equal(t, []byte{0xFF, 0xFF, 0xFF}, got[1:4])                     // sentinel
	require.Equal(t, []byte{0x00, 0x00, 0x06}, got[4:7])                     // full message length
	require.Equal(t, uint32(0x1000000), binary.BigEndian.Uint32(got[12:16])) // ext on type 0
	require.Equal(t, []byte{1, 2, 3, 4}, got[16:20])

	require.Equal(t, byte(3<<6|4), got[20])                                  // fmt 3, chunk id 4
	require.Equal(t, uint32(0x1000000), binary.BigEndian.Uint32(got[21:25])) // ext repeated
	require.Equal(t, []byte{5, 6}, got[25:27])
}

func TestWriteMessageNoExtendedTimestamp(t *testing.T) {
	var buf bytes.Buffer
	c := &Conn{wr: &buf, wrPacketSize: 4096}

	require.NoError(t, c.writeMessage(4, TypeVideo, 1000, []byte{0x01, 0x02}))

	got := buf.Bytes()
	require.Len(t, got, 1+11+2)                          // no extended timestamp
	require.Equal(t, []byte{0x00, 0x03, 0xE8}, got[1:4]) // 1000
	require.Equal(t, []byte{0x01, 0x02}, got[12:])
}

func TestWritePublishPropagatesReadError(t *testing.T) {
	c := &Conn{rd: bytes.NewReader(nil), wr: io.Discard, wrPacketSize: 4096}
	require.Error(t, c.writePublish())
}

func TestWritePlayPropagatesReadError(t *testing.T) {
	c := &Conn{rd: bytes.NewReader(nil), wr: io.Discard, wrPacketSize: 4096}
	require.Error(t, c.writePlay())
}
