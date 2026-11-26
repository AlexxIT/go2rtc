package streams

import (
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// Transcoders convert a consumer's backchannel (e.g. a browser microphone)
// into a codec that only a source accepts, e.g. AAC or AAC-ELD. They are
// named in the config, because only the config can name a program, and used
// automatically: when no source of a stream accepts a backchannel, a
// transcoder is tried whose output codec one of the stream's sources accepts.
// The transcoder plays its output to the stream (Stream.Play).

type transcoder struct {
	name   string
	output *core.Codec
}

var (
	transcoders   []transcoder
	transcodersMu sync.Mutex
)

// AddTranscoder registers a transcoder for all streams. Its producer is
// created with the "transcode:<name>#output=<codec>" source, where codec is
// the one the stream's source takes, and must implement Player. An output
// without clock rate or channels matches any.
func AddTranscoder(name string, output *core.Codec) {
	transcodersMu.Lock()
	transcoders = append(transcoders, transcoder{name: name, output: output})
	transcodersMu.Unlock()
}

// Player is a producer that plays its output to the stream it serves.
type Player interface {
	SetPlay(play func(core.Producer) error)
}

// matchTranscoder adds a backchannel media of the consumer to a transcoder
// whose output one of the stream's sources accepts, and returns it.
func (s *Stream) matchTranscoder(cons core.Consumer, consMedia *core.Media, prodMedias []*core.Media) *Producer {
	transcodersMu.Lock()
	list := transcoders
	transcodersMu.Unlock()

	for _, t := range list {
		output := acceptedCodec(prodMedias, t.output)
		if output == nil {
			continue
		}

		prod := s.transcoder(t.name + "#output=" + output.String())
		if err := prod.Dial(); err != nil {
			log.Warn().Err(err).Str("transcoder", t.name).Msg("[streams] can't start transcoder")
			continue
		}

		prod.mu.Lock()
		player, ok := prod.conn.(Player)
		prod.mu.Unlock()
		if !ok {
			continue
		}
		player.SetPlay(func(prod core.Producer) error { return s.Play(prod) })

		for _, prodMedia := range prod.GetMedias() {
			prodCodec, consCodec := prodMedia.MatchMedia(consMedia)
			if prodCodec == nil {
				continue
			}

			track, err := cons.(core.Producer).GetTrack(consMedia, consCodec)
			if err != nil {
				log.Info().Err(err).Msg("[streams] can't get track")
				continue
			}
			if err = prod.AddTrack(prodMedia, prodCodec, track); err != nil {
				log.Info().Err(err).Msg("[streams] can't add track")
				continue
			}

			log.Debug().Str("transcoder", t.name).Msg("[streams] backchannel through transcoder")
			return prod
		}
	}

	return nil
}

// transcoder returns the stream's producer for a transcoder, one per stream.
func (s *Stream) transcoder(name string) *Producer {
	url := "transcode:" + name

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, prod := range s.transcoders {
		if prod.url == url {
			return prod
		}
	}

	prod := NewProducer(url)
	s.transcoders = append(s.transcoders, prod)
	return prod
}

// acceptedCodec returns the codec of a source that takes backchannel audio
// matching codec, or nil.
func acceptedCodec(medias []*core.Media, codec *core.Codec) *core.Codec {
	for _, media := range medias {
		if media.Direction != core.DirectionSendonly {
			continue
		}
		if accepted := media.MatchCodec(codec); accepted != nil {
			return accepted
		}
	}
	return nil
}
