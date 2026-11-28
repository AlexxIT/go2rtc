package homekit

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/stretchr/testify/require"
)

func TestTrackToVideoWithoutTrack(t *testing.T) {
	video0 := &camera.VideoCodecConfiguration{
		CodecParams: []camera.VideoCodecParameters{{ProfileID: []byte{1}, Level: []byte{2}}},
		VideoAttrs:  []camera.VideoCodecAttributes{{Width: 640, Height: 360, Framerate: 30}, {Width: 1280, Height: 720, Framerate: 30}},
	}

	// audio only consumer: the largest resolution, as for a video consumer
	video := trackToVideo(nil, video0, 0, 0)
	require.Equal(t, []camera.VideoCodecAttributes{{Width: 1280, Height: 720, Framerate: 30}}, video.VideoAttrs)

	video = trackToVideo(nil, video0, 640, 0)
	require.Equal(t, []camera.VideoCodecAttributes{{Width: 640, Height: 360, Framerate: 30}}, video.VideoAttrs)
}
