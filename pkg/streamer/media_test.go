package streamer

import (
	"github.com/pion/sdp/v3"
	"github.com/stretchr/testify/assert"
	"net/url"
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

func TestParseQuery(t *testing.T) {
	u, _ := url.Parse("rtsp://localhost:8554/camera1")
	medias := ParseQuery(u.Query())
	assert.Nil(t, medias)

	for _, rawULR := range []string{
		"rtsp://localhost:8554/camera1?video",
		"rtsp://localhost:8554/camera1?video=copy",
		"rtsp://localhost:8554/camera1?video=any",
	} {
		u, _ = url.Parse(rawULR)
		medias = ParseQuery(u.Query())
		assert.Equal(t, []*Media{
			{Kind: KindVideo, Direction: DirectionRecvonly, Codecs: []*Codec{{Name: CodecAny}}},
		}, medias)
	}
}
