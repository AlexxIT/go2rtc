package homekit

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/stretchr/testify/require"
)

func TestTrackToAudioRTPTime(t *testing.T) {
	conf := &camera.AudioCodecConfiguration{
		CodecType: camera.AudioCodecTypeAACELD,
		CodecParams: []camera.AudioCodecParameters{
			{Channels: 1, SampleRate: []byte{camera.AudioCodecSampleRate16Khz}},
		},
	}

	// AAC-ELD at 16 kHz: 480 samples per frame, 30 ms
	audio := trackToAudio(nil, conf)
	require.Equal(t, []uint8{camera.RTPTimeAACELD16}, audio.CodecParams[0].RTPTime)

	// Opus keeps 20 ms
	track := core.NewReceiver(nil, &core.Codec{Name: core.CodecOpus, ClockRate: 16000, Channels: 1})
	audio = trackToAudio(track, conf)
	require.Equal(t, byte(camera.AudioCodecTypeOpus), audio.CodecType)
	require.Equal(t, []uint8{20}, audio.CodecParams[0].RTPTime)
}
