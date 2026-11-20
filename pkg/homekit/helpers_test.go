package homekit

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/stretchr/testify/require"
)

func TestTrackToVideoAudioOnly(t *testing.T) {
	video0 := &camera.VideoCodecConfiguration{
		CodecParams: []camera.VideoCodecParameters{
			{ProfileID: []byte{0, 1, 2}, Level: []byte{0, 1, 2}},
		},
		VideoAttrs: []camera.VideoCodecAttributes{
			{Width: 1920, Height: 1080, Framerate: 30},
			{Width: 1280, Height: 720, Framerate: 30},
			{Width: 640, Height: 360, Framerate: 30},
		},
	}

	// no video track should not request an empty resolution
	codec := trackToVideo(nil, video0, 0, 0)
	require.Equal(t, []byte{2}, codec.CodecParams[0].ProfileID)
	require.Equal(t, []byte{2}, codec.CodecParams[0].Level)
	require.Equal(t, camera.VideoCodecAttributes{Width: 1920, Height: 1080, Framerate: 30}, codec.VideoAttrs[0])

	codec = trackToVideo(nil, video0, 1280, 0)
	require.Equal(t, camera.VideoCodecAttributes{Width: 1280, Height: 720, Framerate: 30}, codec.VideoAttrs[0])
}
