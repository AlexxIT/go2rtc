package exec

import (
	"errors"
	"io"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/pion/rtp"
)

// transcoder is a backchannel that sends the audio it receives (e.g. a
// browser's microphone) through an external encoder, and plays the
// encoder's output to the stream. It lets a microphone talk to cameras that
// take only a codec browsers can't send. Transcoders are named in the config,
// because only the config can name a program:
//
//	transcode:
//	  aac: ffmpeg -f s16le -ar 16000 -ac 1 -i - -c:a aac -f adts -#audio=pcml/16000#output=aac/16000
//
// and used automatically by every stream with a source that takes the output
// codec, when no source takes the microphone's codec (see streams.Player).
//
// The encoder reads audio in the "audio" codec on stdin (G.711 from the
// microphone is converted to PCM if needed) and writes any format magic.Open
// reads, e.g. ADTS, or LOAS for AAC-ELD, to stdout.
type transcoder struct {
	core.Connection
	cmd   *shell.Command
	input *core.Codec
	play  func(core.Producer) error

	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func newTranscoder(cmd *shell.Command, audio string) (*transcoder, error) {
	input := &core.Codec{Name: core.CodecPCML, ClockRate: 16000}
	if audio != "" {
		if input = core.ParseCodecString(audio); input == nil {
			return nil, errors.New("exec: unsupported audio format: " + audio)
		}
	}

	codecs := []*core.Codec{input}
	switch input.Name {
	case core.CodecPCM, core.CodecPCML:
		// microphones in G.711 are converted to the encoder's PCM input
		codecs = append(codecs,
			&core.Codec{Name: core.CodecPCMA, ClockRate: 8000},
			&core.Codec{Name: core.CodecPCMU, ClockRate: 8000},
		)
	}

	return &transcoder{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "transcoder",
			Protocol:   "pipe",
			Medias: []*core.Media{
				{Kind: core.KindAudio, Direction: core.DirectionSendonly, Codecs: codecs},
			},
			Transport: cmd,
		},
		cmd:   cmd,
		input: input,
	}, nil
}

// SetPlay sets where the encoder's output goes: the stream (streams.Player).
func (t *transcoder) SetPlay(play func(core.Producer) error) {
	t.play = play
}

func (t *transcoder) GetTrack(*core.Media, *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (t *transcoder) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	if t.stdin == nil {
		var err error
		if t.stdin, err = t.cmd.StdinPipe(); err != nil {
			return err
		}
		if t.stdout, err = t.cmd.StdoutPipe(); err != nil {
			return err
		}
	}

	convert := func(b []byte) []byte { return b }
	if track.Codec.Name != t.input.Name || track.Codec.ClockRate != t.input.ClockRate {
		convert = pcm.Transcode(t.input, track.Codec)
	}

	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		if n, err := t.stdin.Write(convert(packet.Payload)); err == nil {
			t.Send += n
		}
	}
	sender.HandleRTP(track)
	t.Senders = append(t.Senders, sender)
	return nil
}

func (t *transcoder) Start() error {
	if t.stdin == nil {
		return errors.New("exec: transcoder without tracks")
	}

	if err := t.cmd.Start(); err != nil {
		return err
	}

	go func() {
		prod, err := magic.Open(t.stdout)
		if err != nil {
			log.Warn().Err(err).Msg("[exec] transcoder output")
			return
		}

		if t.play == nil {
			_ = prod.Stop()
			return
		}
		if err = t.play(prod); err != nil {
			log.Warn().Err(err).Msg("[exec] transcoder")
		}
	}()

	return t.cmd.Wait()
}

// transcodeSources returns a transcoder's command and params for the codec
// the stream's source takes.
var transcodeSources = map[string]func(output *core.Codec) string{}

// AddTranscoder registers a transcoder from code, e.g. the ffmpeg module's
// built-in ones. source returns the command and its params (#audio=) for the
// codec the stream's source takes, which matches output.
func AddTranscoder(name string, output *core.Codec, source func(output *core.Codec) string) {
	transcodeSources[name] = source
	streams.AddTranscoder(name, output)
}

// initTranscoders registers the transcoders named in the config. They are
// tried before built-in ones.
func initTranscoders() {
	var cfg struct {
		Mod map[string]string `yaml:"transcode"`
	}
	app.LoadConfig(&cfg)

	for name, source := range cfg.Mod {
		_, rawQuery, _ := strings.Cut(source, "#")
		output := core.ParseCodecString(streams.ParseQuery(rawQuery).Get("output"))
		if output == nil {
			log.Warn().Str("transcoder", name).Msg("[exec] transcoder needs #output=<codec>")
			continue
		}
		AddTranscoder(name, output, func(*core.Codec) string { return source })
	}

	// Sources can only name a registered transcoder, so the API may add them
	streams.HandleFunc("transcode", transcodeHandle)
}

func transcodeHandle(rawURL string) (core.Producer, error) {
	name, rawQuery, _ := strings.Cut(rawURL[len("transcode:"):], "#")
	source, ok := transcodeSources[name]
	if !ok {
		return nil, errors.New("exec: unknown transcoder: " + name)
	}

	output := core.ParseCodecString(streams.ParseQuery(rawQuery).Get("output"))
	if output == nil {
		return nil, errors.New("exec: transcoder without output: " + rawURL)
	}

	cmdline, rawQuery, _ := strings.Cut(source(output), "#")
	cmd := shell.NewCommand(cmdline)
	cmd.Stderr = &logWriter{buf: make([]byte, 512), debug: log.Debug().Enabled()}

	return newTranscoder(cmd, streams.ParseQuery(rawQuery).Get("audio"))
}
