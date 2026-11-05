package streams

import (
	"net/url"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestRecursion(t *testing.T) {
	// create stream with some source
	stream1, err := New("from_yaml", "does_not_matter")
	require.NoError(t, err)
	require.Len(t, streams, 1)

	// ask another unnamed stream that links go2rtc
	query, err := url.ParseQuery("src=rtsp://localhost:8554/from_yaml?video")
	require.NoError(t, err)
	stream2, err := GetOrPatch(query)
	require.NoError(t, err)

	// check stream is same
	require.Equal(t, stream1, stream2)
	// check stream urls is same
	require.Equal(t, stream1.producers[0].url, stream2.producers[0].url)
	require.Len(t, streams, 2)
}

func TestTempate(t *testing.T) {
	HandleFunc("rtsp", func(url string) (core.Producer, error) { return nil, nil }) // bypass HasProducer

	// config from yaml
	stream1, err := New("camera.from_hass", "ffmpeg:{input}#video=copy")
	require.NoError(t, err)
	// request from hass
	stream2, err := Patch("camera.from_hass", "rtsp://example.com")
	require.NoError(t, err)

	require.Equal(t, stream1, stream2)
	require.Equal(t, "ffmpeg:rtsp://example.com#video=copy", stream1.producers[0].url)
}

type mockConsumer struct {
	medias []*core.Media
	tracks []*core.Receiver
}

func (m *mockConsumer) GetMedias() []*core.Media {
	return m.medias
}

func (m *mockConsumer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	m.tracks = append(m.tracks, track)
	return nil
}

func (m *mockConsumer) Stop() error {
	return nil
}

type mockProducer struct {
	medias []*core.Media
}

func (m *mockProducer) GetMedias() []*core.Media {
	return m.medias
}

func (m *mockProducer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return core.NewReceiver(media, codec), nil
}

func (m *mockProducer) Start() error {
	return nil
}

func (m *mockProducer) Stop() error {
	return nil
}

func TestAddConsumerNoDegradedAudioOnlyOnVideoDialError(t *testing.T) {
	var primaryCalls, ffmpegCalls int

	HandleFunc("reolink", func(rawURL string) (core.Producer, error) {
		primaryCalls++
		if primaryCalls == 1 {
			return nil, assertError("dial timeout")
		}
		return &mockProducer{
			medias: []*core.Media{
				{
					Kind:      core.KindVideo,
					Direction: core.DirectionRecvonly,
					Codecs:    []*core.Codec{{Name: core.CodecH265, ClockRate: 90000}},
				},
				{
					Kind:      core.KindAudio,
					Direction: core.DirectionRecvonly,
					Codecs:    []*core.Codec{{Name: core.CodecAAC, ClockRate: 16000}},
				},
			},
		}, nil
	})

	HandleFunc("ffmpeg", func(rawURL string) (core.Producer, error) {
		ffmpegCalls++
		return &mockProducer{
			medias: []*core.Media{
				{
					Kind:      core.KindAudio,
					Direction: core.DirectionRecvonly,
					Codecs:    []*core.Codec{{Name: core.CodecOpus, ClockRate: 48000}},
				},
			},
		}, nil
	})

	stream := NewStream([]string{
		"reolink://camera/main",
		"ffmpeg:driveway_test#audio=opus",
	})

	cons := &mockConsumer{
		medias: []*core.Media{
			{
				Kind:      core.KindVideo,
				Direction: core.DirectionSendonly,
				Codecs:    []*core.Codec{{Name: core.CodecAny}},
			},
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs:    []*core.Codec{{Name: core.CodecAny}},
			},
		},
	}

	// First attempt: primary producer fails to dial during video check.
	// AddConsumer must fail rather than dialing ffmpeg:#audio=opus and returning an audio-only stream.
	err := stream.AddConsumer(cons)
	require.Error(t, err)
	require.Equal(t, 1, primaryCalls)
	require.Equal(t, 0, ffmpegCalls)
	require.Empty(t, cons.tracks)

	// Second attempt: primary producer succeeds and provides both video and audio without dialing ffmpeg.
	err = stream.AddConsumer(cons)
	require.NoError(t, err)
	require.Equal(t, 2, primaryCalls)
	require.Equal(t, 0, ffmpegCalls)
	require.Len(t, cons.tracks, 2)
}

type assertError string

func (e assertError) Error() string { return string(e) }

