package core

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/pion/sdp/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			{Kind: KindVideo, Direction: DirectionSendonly, Codecs: []*Codec{{Name: CodecAny}}},
		}, medias)
	}
}

func TestClone(t *testing.T) {
	media1 := &Media{
		Kind:      KindVideo,
		Direction: DirectionRecvonly,
		Codecs: []*Codec{
			{Name: CodecPCMU, ClockRate: 8000},
		},
	}
	media2 := media1.Clone()

	p1 := fmt.Sprintf("%p", media1)
	p2 := fmt.Sprintf("%p", media2)
	require.NotEqualValues(t, p1, p2)

	p3 := fmt.Sprintf("%p", media1.Codecs[0])
	p4 := fmt.Sprintf("%p", media2.Codecs[0])
	require.NotEqualValues(t, p3, p4)
}
