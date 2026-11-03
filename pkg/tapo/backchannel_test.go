package tapo

import (
	"bytes"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestBackchannelPTS(t *testing.T) {
	// RTP timestamps start at a random value, including near the uint32 end
	for _, start := range []uint32{1000, 381_000_000, 2_200_000_000, 4294966500} {
		testBackchannelPTS(t, start)
	}
}

func testBackchannelPTS(t *testing.T, start uint32) {
	// PCMA/8000, 40 ms per packet = 320 samples = 320 RTP ticks = 3600 PTS ticks
	const samples = 320

	muxer := mpegts.NewMuxer()
	pid := muxer.AddTrack(mpegts.StreamTypePCMATapo)

	buf := bytes.NewBuffer(muxer.GetHeader())

	codec := &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
	handler := backchannelHandler(muxer, pid, codec, func(b []byte) {
		buf.Write(b)
	})

	// the demuxer returns a PES only when the next one starts, so send one extra
	const count = 4
	for i := 0; i < count+1; i++ {
		handler(&rtp.Packet{
			Header:  rtp.Header{Timestamp: start + uint32(i)*samples},
			Payload: make([]byte, samples),
		})
	}

	demux := mpegts.NewDemuxer()
	_, err := demux.ReadPacket(buf) // PMT
	require.NoError(t, err)

	var pts []uint32
	for i := 0; i < count; i++ {
		pkt, err := demux.ReadPacket(buf)
		require.NoError(t, err)
		pts = append(pts, pkt.Timestamp)
	}

	for i := 1; i < len(pts); i++ {
		require.Equal(t, uint32(3600), pts[i]-pts[i-1], "PTS step between packets %d and %d", i-1, i)
	}
}
