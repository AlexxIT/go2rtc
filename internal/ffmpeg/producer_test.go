package ffmpeg

import (
	"encoding/json"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestPresetCodec(t *testing.T) {
	tests := []struct {
		preset string
		expect *core.Codec
	}{
		{"opus", &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}},
		{"opus/16000", &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}},
		{"pcma", &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}},
		{"pcmu/16000", &core.Codec{Name: core.CodecPCMU, ClockRate: 16000}},
		{"pcm/48000", &core.Codec{Name: core.CodecPCM, ClockRate: 48000}},
		{"pcml/44100", &core.Codec{Name: core.CodecPCML, ClockRate: 44100}},
		{"aac/16000", &core.Codec{Name: core.CodecAAC, ClockRate: 16000, FmtpLine: aac.FMTP + "1408"}},
		{"aac", nil},        // keeps source sample rate
		{"mp3", nil},        // unknown output params
		{"h264", nil},       // not audio
		{"pcma/22050", nil}, // no preset
	}
	for _, test := range tests {
		t.Run(test.preset, func(t *testing.T) {
			require.Equal(t, test.expect, presetCodec(test.preset))
		})
	}
}

func TestNewProducerPreset(t *testing.T) {
	prod, err := NewProducer("ffmpeg:camera1#audio=opus#query=log_level=debug")
	require.NoError(t, err)

	medias := prod.GetMedias()
	require.Equal(t, []*core.Codec{{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}}, medias[0].Codecs)

	// streams API clients (ex. Home Assistant) expect url for every producer
	b, err := json.Marshal(prod)
	require.NoError(t, err)
	require.Contains(t, string(b), `"url":"ffmpeg:camera1#audio=opus#query=log_level=debug"`)

	_, err = NewProducer("ffmpeg:camera1#audio=aac")
	require.Error(t, err)
}

type testProducer struct {
	core.Connection
	done chan struct{}
}

func (p *testProducer) Start() error {
	<-p.done
	return nil
}

func (p *testProducer) Stop() error {
	close(p.done)
	return nil
}

type testConsumer struct {
	core.Connection
}

func (c *testConsumer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	c.Senders = append(c.Senders, core.NewSender(media, codec).WithParent(track))
	return nil
}

func TestIsLazyAudio(t *testing.T) {
	streams.HandleFunc("ffmpeg", NewProducer) // bypass HasProducer
	streams.HandleFunc("test", func(url string) (core.Producer, error) {
		prod := &testProducer{done: make(chan struct{})}
		prod.Medias = []*core.Media{
			{Kind: core.KindVideo, Direction: core.DirectionRecvonly, Codecs: []*core.Codec{{Name: core.CodecH264}}},
		}
		if url == "test:audio" {
			prod.Medias = append(prod.Medias, &core.Media{
				Kind: core.KindAudio, Direction: core.DirectionRecvonly,
				Codecs: []*core.Codec{{Name: core.CodecAAC, ClockRate: 16000}},
			})
		}
		return prod, nil
	})

	dial := func(name, source string) {
		stream, err := streams.New(name, source, "ffmpeg:"+name+"#audio=opus")
		require.NoError(t, err)

		cons := &testConsumer{}
		cons.Medias = []*core.Media{
			{Kind: core.KindVideo, Direction: core.DirectionSendonly, Codecs: []*core.Codec{{Name: core.CodecH264}}},
		}
		require.NoError(t, stream.AddConsumer(cons))
		t.Cleanup(func() { stream.RemoveConsumer(cons) })
	}

	// source not dialed yet
	_, err := streams.New("lazy_none", "test:audio")
	require.NoError(t, err)
	require.False(t, isLazyAudio("ffmpeg:lazy_none#audio=opus"))

	dial("lazy_audio", "test:audio")
	require.True(t, isLazyAudio("ffmpeg:lazy_audio#audio=opus"))
	require.True(t, isLazyAudio("ffmpeg:lazy_audio#audio=pcma#query=log_level=debug"))
	require.False(t, isLazyAudio("ffmpeg:lazy_audio#audio=aac"))
	require.False(t, isLazyAudio("ffmpeg:lazy_audio#video=h264#audio=opus"))
	require.False(t, isLazyAudio("ffmpeg:lazy_audio#audio=opus#raw=-ar 22050"))
	require.False(t, isLazyAudio("ffmpeg:lazy_audio"))

	dial("lazy_video", "test:video")
	require.False(t, isLazyAudio("ffmpeg:lazy_video#audio=opus"))

	require.False(t, isLazyAudio("ffmpeg:rtsp://example.com/stream#audio=opus"))
}
