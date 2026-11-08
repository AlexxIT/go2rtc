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

type testProducer struct {
	core.Connection
}

func (p *testProducer) Start() error { return nil }

func TestHasAudio(t *testing.T) {
	video := &core.Media{Kind: core.KindVideo, Direction: core.DirectionRecvonly}
	audio := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly}
	backchannel := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly}

	newStream := func(medias ...*core.Media) *Stream {
		prod := &testProducer{}
		prod.Medias = medias
		return &Stream{producers: []*Producer{
			{url: "rtsp://example.com", conn: prod, state: stateMedias},
			{url: "ffmpeg:camera1#audio=opus"},
		}}
	}

	require.True(t, newStream(video, audio).HasAudio("ffmpeg:camera1#audio=opus"))
	require.False(t, newStream(video).HasAudio("ffmpeg:camera1#audio=opus"))
	require.False(t, newStream(video, backchannel).HasAudio("ffmpeg:camera1#audio=opus"))
	// producer with skip source is not checked
	require.False(t, newStream(video, audio).HasAudio("rtsp://example.com"))
	// producer is not dialed yet
	require.False(t, (&Stream{producers: []*Producer{{url: "rtsp://example.com"}}}).HasAudio(""))
}
