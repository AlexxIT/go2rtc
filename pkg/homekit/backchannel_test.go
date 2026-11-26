package homekit

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
	"github.com/stretchr/testify/require"
)

func audioConfig(codecType byte, rates ...byte) camera.AudioCodecConfiguration {
	return camera.AudioCodecConfiguration{
		CodecType:   codecType,
		CodecParams: []camera.AudioCodecParameters{{Channels: 1, SampleRate: rates}},
	}
}

func TestBackchannelMedia(t *testing.T) {
	withSpeaker := &hap.Accessory{Services: []*hap.Service{{Type: "110"}, {Type: typeSpeaker}}}
	withoutSpeaker := &hap.Accessory{Services: []*hap.Service{{Type: "110"}}}

	eld := []camera.AudioCodecConfiguration{audioConfig(camera.AudioCodecTypeAACELD, camera.AudioCodecSampleRate16Khz)}
	both := []camera.AudioCodecConfiguration{
		audioConfig(camera.AudioCodecTypeOpus, camera.AudioCodecSampleRate16Khz, camera.AudioCodecSampleRate24Khz),
		audioConfig(camera.AudioCodecTypeAACELD, camera.AudioCodecSampleRate16Khz),
	}

	require.Nil(t, backchannelMedia(withoutSpeaker, eld))

	// AAC-ELD only (e.g. Aqara G410): the camera's own codec, nothing else
	media := backchannelMedia(withSpeaker, eld)
	require.Equal(t, core.DirectionSendonly, media.Direction)
	require.Len(t, media.Codecs, 1)
	require.Equal(t, core.CodecELD, media.Codecs[0].Name)
	require.Equal(t, uint32(16000), media.Codecs[0].ClockRate)

	// Opus once, with the 48 kHz RTP clock a WebRTC microphone uses
	media = backchannelMedia(withSpeaker, both)
	require.Len(t, media.Codecs, 2)
	require.Equal(t, &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}, media.Codecs[0])
	require.NotNil(t, media.MatchCodec(&core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}))
	require.Nil(t, media.MatchCodec(&core.Codec{Name: core.CodecPCMA, ClockRate: 8000}))
}

func TestBackchannelToAudio(t *testing.T) {
	codecs := []camera.AudioCodecConfiguration{
		audioConfig(camera.AudioCodecTypeAACELD, camera.AudioCodecSampleRate16Khz),
		audioConfig(camera.AudioCodecTypeOpus, camera.AudioCodecSampleRate24Khz, camera.AudioCodecSampleRate16Khz),
	}

	audio := backchannelToAudio(core.CodecOpus, codecs)
	require.Equal(t, byte(camera.AudioCodecTypeOpus), audio.CodecType)
	require.Equal(t, []byte{camera.AudioCodecSampleRate16Khz}, audio.CodecParams[0].SampleRate)
	require.Equal(t, []uint8{20}, audio.CodecParams[0].RTPTime)

	audio = backchannelToAudio(core.CodecELD, codecs)
	require.Equal(t, byte(camera.AudioCodecTypeAACELD), audio.CodecType)
	require.Equal(t, []uint8{camera.RTPTimeAACELD16}, audio.CodecParams[0].RTPTime)

	// the camera's list isn't changed
	require.Len(t, codecs[1].CodecParams[0].SampleRate, 2)

	require.Nil(t, backchannelToAudio(core.CodecPCMA, codecs))
}

func TestBackchannelAfterStart(t *testing.T) {
	session := &srtp.Session{}
	c := &Client{}

	// live view started with AAC-ELD, then a microphone is added
	c.startedAudio = session
	c.startedCodec = core.CodecELD

	media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly}
	codec := &core.Codec{Name: core.CodecELD, ClockRate: 16000, Channels: 1}
	track := core.NewReceiver(media, &core.Codec{Name: core.CodecELD, ClockRate: 16000, Channels: 1, PayloadType: core.PayloadTypeRAW})
	require.NoError(t, c.AddTrack(media, codec, track))
	require.Same(t, session, c.backchannelSession.Load())

	// another codec than the stream's isn't sent
	c = &Client{startedAudio: session, startedCodec: core.CodecOpus}
	require.NoError(t, c.AddTrack(media, codec, track))
	require.Nil(t, c.backchannelSession.Load())

	require.NoError(t, c.Stop())
}
