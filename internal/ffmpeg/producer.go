package ffmpeg

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Producer struct {
	core.Connection
	url    string
	query  url.Values
	source string
	ffmpeg core.Producer
	closed bool
	mu     sync.Mutex
}

// NewProducer - FFmpeg producer with auto selection video/audio codec based on client capabilities
// or with known audio codec from preset. FFmpeg starts only after consumers get their tracks.
func NewProducer(url string) (core.Producer, error) {
	p := &Producer{source: url}

	i := strings.IndexByte(url, '#')
	p.url, p.query = url[:i], streams.ParseQuery(url[i+1:])

	// ffmpeg.NewProducer support only one audio
	if len(p.query["video"]) != 0 || len(p.query["audio"]) != 1 {
		return nil, errors.New("ffmpeg: unsupported params: " + url[i:])
	}

	p.ID = core.NewID()
	p.FormatName = "ffmpeg"
	p.URL = url // stream info should have the producer url until FFmpeg starts
	p.Medias = []*core.Media{
		{
			// we can support only audio, because don't know FmtpLine for H264 and PayloadType for MJPEG
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			// codecs in order from best to worst
			Codecs: []*core.Codec{
				// OPUS will always marked as OPUS/48000/2
				{Name: core.CodecOpus, ClockRate: 48000, Channels: 2},
				{Name: core.CodecPCML, ClockRate: 16000},
				{Name: core.CodecPCM, ClockRate: 16000},
				{Name: core.CodecPCMA, ClockRate: 16000},
				{Name: core.CodecPCMU, ClockRate: 16000},
				{Name: core.CodecPCML, ClockRate: 8000},
				{Name: core.CodecPCM, ClockRate: 8000},
				{Name: core.CodecPCMA, ClockRate: 8000},
				{Name: core.CodecPCMU, ClockRate: 8000},
				// AAC has unknown problems on Dahua two way
				{Name: core.CodecAAC, ClockRate: 16000, FmtpLine: aac.FMTP + "1408"},
			},
		},
	}

	if audio := p.query.Get("audio"); audio != "auto" {
		codec := presetCodec(audio)
		if codec == nil {
			return nil, errors.New("ffmpeg: unsupported params: " + url[i:])
		}
		p.Medias[0].Codecs = []*core.Codec{codec}
	}

	return p, nil
}

// isLazyAudio - audio transcoding of the go2rtc stream, that already has audio.
// Output codec is known from preset, so consumers don't need to wait for FFmpeg start.
func isLazyAudio(source string) bool {
	input, rawQuery, ok := strings.Cut(source[7:], "#") // remove `ffmpeg:`
	if !ok {
		return false
	}

	query := streams.ParseQuery(rawQuery)
	if len(query["video"]) != 0 || len(query["audio"]) != 1 || query["raw"] != nil {
		return false
	}
	if presetCodec(query.Get("audio")) == nil {
		return false
	}

	stream := streams.Get(input)
	return stream != nil && stream.HasAudio(source)
}

// presetCodec - return output codec for audio preset or nil if it can't be known before FFmpeg start
func presetCodec(preset string) *core.Codec {
	if defaults[preset] == "" {
		return nil
	}

	name, rate, _ := strings.Cut(preset, "/")
	if rate == "" {
		rate = "8000"
	}

	switch name {
	case "opus":
		// OPUS will always marked as OPUS/48000/2
		return &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}
	case "aac":
		// AAC without rate keeps source sample rate
		if rate == "16000" {
			return &core.Codec{Name: core.CodecAAC, ClockRate: 16000, FmtpLine: aac.FMTP + "1408"}
		}
	case "pcm":
		return &core.Codec{Name: core.CodecPCM, ClockRate: uint32(core.Atoi(rate))}
	case "pcml":
		return &core.Codec{Name: core.CodecPCML, ClockRate: uint32(core.Atoi(rate))}
	case "pcma":
		return &core.Codec{Name: core.CodecPCMA, ClockRate: uint32(core.Atoi(rate))}
	case "pcmu":
		return &core.Codec{Name: core.CodecPCMU, ClockRate: uint32(core.Atoi(rate))}
	}

	return nil
}

func (p *Producer) Start() error {
	source := p.source
	if p.query.Get("audio") == "auto" {
		source = p.newURL()
	}

	// skip FFmpeg redirect, because it can return this lazy producer again
	prod, err := streams.GetProducer("exec:" + parseArgs(source[7:]).String())
	if err != nil {
		return err
	}

	p.mu.Lock()
	if p.closed {
		// producer was stopped while FFmpeg was starting
		p.mu.Unlock()
		return prod.Stop()
	}
	p.ffmpeg = prod
	p.mu.Unlock()

	for i, media := range prod.GetMedias() {
		track, err := prod.GetTrack(media, media.Codecs[0])
		if err != nil {
			return err
		}
		// link instead of replace, because next consumers will get our receiver from the stream
		track.AppendChild(&p.Receivers[i].Node)
	}

	return prod.Start()
}

func (p *Producer) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true

	if p.ffmpeg == nil {
		return nil
	}
	return p.ffmpeg.Stop()
}

func (p *Producer) MarshalJSON() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ffmpeg == nil {
		return json.Marshal(p.Connection)
	}
	return json.Marshal(p.ffmpeg)
}

func (p *Producer) newURL() string {
	s := p.url
	// rewrite codecs in url from auto to known presets from defaults
	for _, receiver := range p.Receivers {
		codec := receiver.Codec
		switch codec.Name {
		case core.CodecOpus:
			s += "#audio=opus/16000"
		case core.CodecAAC:
			s += "#audio=aac/16000"
		case core.CodecPCML:
			s += "#audio=pcml/" + strconv.Itoa(int(codec.ClockRate))
		case core.CodecPCM:
			s += "#audio=pcm/" + strconv.Itoa(int(codec.ClockRate))
		case core.CodecPCMA:
			s += "#audio=pcma/" + strconv.Itoa(int(codec.ClockRate))
		case core.CodecPCMU:
			s += "#audio=pcmu/" + strconv.Itoa(int(codec.ClockRate))
		}
	}
	// add other params
	for key, values := range p.query {
		if key != "audio" {
			for _, value := range values {
				s += "#" + key + "=" + value
			}
		}
	}

	return s
}
