package streamer

import (
	"github.com/pion/sdp/v3"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSDP(t *testing.T) {
	medias := []*Media{{
		Kind: KindAudio, Direction: DirectionSendonly,
		Codecs: []*Codec{
			{Name: CodecPCMU, ClockRate: 8000},
		},
	}}

	data, err := MarshalSDP("go2rtc/1.0.0", medias)
	assert.Empty(t, err)

	sd := &sdp.SessionDescription{}
	err = sd.Unmarshal(data)
	assert.Empty(t, err)
}
