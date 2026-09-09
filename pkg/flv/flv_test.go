package flv

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
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
	for range 32 {
		// 1000ms/(90000/4500) = 50ms
		require.Equal(t, uint32(frameN*4500), TimeToRTP(uint32(frameN*50), 90000))
		// 1000ms/(16000/1024) = 64ms
		require.Equal(t, uint32(frameN*1024), TimeToRTP(uint32(frameN*64), 16000))
		frameN *= 2
	}
}
