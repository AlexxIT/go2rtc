package flv

import (
	"bytes"
	"io"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func tagTimeMS(tag []byte) uint32 {
	return uint32(tag[4])<<16 | uint32(tag[5])<<8 | uint32(tag[6]) | uint32(tag[7])<<24
}

// A first packet at timestamp 0 must still latch the baseline, so later packets
// keep real timestamps instead of re-baselining to 0.
func TestPayloaderFirstPacketZeroTimestamp(t *testing.T) {
	m := &Muxer{}
	pay := m.GetPayloader(&core.Codec{Name: core.CodecH264, ClockRate: 90000})

	idr := []byte{0, 0, 0, 1, 0x65}    // AVCC IDR (keyframe)
	pframe := []byte{0, 0, 0, 1, 0x41} // AVCC non-keyframe

	// 15 fps @ 90 kHz = 6000 ticks/frame
	cases := []struct {
		ts      uint32
		payload []byte
		wantMS  uint32
	}{
		{0, idr, 0},
		{6000, pframe, 66}, // old ts0 == 0 sentinel wrongly re-baselined here
		{12000, pframe, 133},
		{18000, pframe, 200},
	}
	for i, c := range cases {
		tag := pay(&rtp.Packet{Header: rtp.Header{Timestamp: c.ts}, Payload: c.payload})
		require.Equal(t, c.wantMS, tagTimeMS(tag), "packet %d", i)
	}
}

func TestTimeToRTP(t *testing.T) {
	// Reolink camera has 20 FPS
	// Video timestamp increases by 50ms, SampleRate 90000, RTP timestamp increases by 4500
	// Audio timestamp increases by 64ms, SampleRate 16000, RTP timestamp increases by 1024
	frameN := 1
	for i := 0; i < 32; i++ {
		// 1000ms/(90000/4500) = 50ms
		require.Equal(t, uint32(frameN*4500), TimeToRTP(uint32(frameN*50), 90000))
		// 1000ms/(16000/1024) = 64ms
		require.Equal(t, uint32(frameN*1024), TimeToRTP(uint32(frameN*64), 16000))
		frameN *= 2
	}
}

// Some RTMP servers send a tag with zero data size before the codec headers.
// probe and Start read the payload, so readPacket must skip it.
func TestReadPacketSkipsEmptyTag(t *testing.T) {
	for _, tagType := range []byte{TagAudio, TagVideo, TagData} {
		var b []byte
		b = append(b, 0, 0, 0, 0)                            // previous tag size
		b = append(b, tagType, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0) // empty tag
		b = append(b, 0, 0, 0, 11)                           // previous tag size
		b = append(b, TagVideo, 0, 0, 2, 0x01, 0x02, 0x03, 0x04, 0, 0, 0)
		b = append(b, 0x17, 0x00)

		c := &Producer{rd: core.NewReadBuffer(bytes.NewReader(b))}
		pkt, err := c.readPacket()
		require.NoError(t, err)
		require.Equal(t, uint8(TagVideo), pkt.PayloadType)
		require.Equal(t, uint32(0x04010203), pkt.Timestamp)
		require.Equal(t, []byte{0x17, 0x00}, pkt.Payload)
	}
}

func TestStartSkipsEmptyVideoFrames(t *testing.T) {
	header := []byte{0x17, PacketTypeAVCNALU, 0, 0, 0}
	idr := h264.JoinNALU([]byte{0x65, 0x88})
	var stream []byte
	for _, payload := range [][]byte{
		header[:2],
		header,
		append(bytes.Clone(header), 0, 0, 0),
		append(append(bytes.Clone(header), 0, 0, 0, 0), idr...),
	} {
		stream = append(stream, 0, 0, 0, 0) // previous tag size
		stream = append(stream, TagVideo, 0, 0, byte(len(payload)), 0, 0, 10, 0, 0, 0, 0)
		stream = append(stream, payload...)
	}

	track := core.NewReceiver(&core.Media{Kind: core.KindVideo}, &core.Codec{Name: core.CodecH264, ClockRate: 90000})
	var packets []*core.Packet
	track.Input = func(packet *core.Packet) {
		packets = append(packets, packet)
	}
	prod := &Producer{rd: core.NewReadBuffer(bytes.NewReader(stream)), video: track}

	require.ErrorIs(t, prod.Start(), io.EOF)
	require.Len(t, packets, 1)
	require.Equal(t, idr, packets[0].Payload)
	require.Equal(t, uint32(900), packets[0].Timestamp)
}
