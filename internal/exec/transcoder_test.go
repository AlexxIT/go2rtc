package exec

import (
	"runtime"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// camera takes AAC-ELD only, like a HomeKit doorbell
type camera struct {
	core.Connection
	tracks chan *core.Receiver
	done   chan struct{}
}

func (c *camera) GetTrack(*core.Media, *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (c *camera) AddTrack(_ *core.Media, _ *core.Codec, track *core.Receiver) error {
	c.tracks <- track
	return nil
}

func (c *camera) Start() error { <-c.done; return nil }
func (c *camera) Stop() error {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	return nil
}

// microphone is a consumer with a G.711 backchannel, like a WebRTC client
type microphone struct {
	core.Connection
}

func (m *microphone) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return core.NewReceiver(media, codec), nil
}

func (m *microphone) AddTrack(*core.Media, *core.Codec, *core.Receiver) error { return nil }
func (m *microphone) Start() error                                            { return nil }

func TestTranscoderAutomatic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	log = zerolog.Nop()

	cam := &camera{
		Connection: core.Connection{Medias: []*core.Media{{
			Kind: core.KindAudio, Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{{Name: core.CodecELD, ClockRate: 16000, Channels: 1}},
		}}},
		tracks: make(chan *core.Receiver, 1),
		done:   make(chan struct{}),
	}
	streams.HandleFunc("testcam", func(string) (core.Producer, error) { return cam, nil })

	// as in the config: a stand-in encoder that "encodes" by writing a ready
	// LOAS file and reads the microphone until it ends
	transcodeCommands["testeld"] = "sh -c 'cat ../../pkg/aac/testdata/eld_16k_480.loas; cat > /dev/null'#audio=pcml/16000#output=eld/16000"
	streams.AddTranscoder("testeld", core.ParseCodecString("eld/16000"))
	streams.HandleFunc("transcode", transcodeHandle)

	stream, err := streams.New("transcoder_test", "testcam:")
	require.NoError(t, err)

	mic := &microphone{Connection: core.Connection{Medias: []*core.Media{{
		Kind: core.KindAudio, Direction: core.DirectionRecvonly,
		Codecs: []*core.Codec{{Name: core.CodecPCMA, ClockRate: 8000}},
	}}}}

	// the camera doesn't take G.711, so the transcoder is used automatically
	require.NoError(t, stream.AddConsumer(mic))

	select {
	case track := <-cam.tracks:
		require.Equal(t, core.CodecELD, track.Codec.Name)
	case <-time.After(5 * time.Second):
		t.Fatal("the camera got no track")
	}

	stream.RemoveConsumer(mic)
}

func TestTranscoderAudio(t *testing.T) {
	_, err := newTranscoder(shell.NewCommand("true"), "nope")
	require.Error(t, err)

	tr, err := newTranscoder(shell.NewCommand("true"), "pcma")
	require.NoError(t, err)
	require.Len(t, tr.GetMedias()[0].Codecs, 1) // no conversion to G.711

	tr, err = newTranscoder(shell.NewCommand("true"), "")
	require.NoError(t, err)
	// PCM input also takes G.711 microphones
	media := tr.GetMedias()[0]
	require.NotNil(t, media.MatchCodec(&core.Codec{Name: core.CodecPCMA, ClockRate: 8000}))
	require.Nil(t, media.MatchCodec(&core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}))
}

func TestTranscoderUnknown(t *testing.T) {
	_, err := transcodeHandle("transcode:missing")
	require.Error(t, err)
}
