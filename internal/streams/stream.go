package streams

import (
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Stream struct {
	producers []*Producer
	consumers []core.Consumer
	mu        sync.Mutex
	pending   atomic.Int32
}

func NewStream(source any) *Stream {
	switch source := source.(type) {
	case string:
		return &Stream{
			producers: []*Producer{NewProducer(source)},
		}
	case []string:
		s := new(Stream)
		for _, str := range source {
			s.producers = append(s.producers, NewProducer(str))
		}
		return s
	case []any:
		s := new(Stream)
		for _, src := range source {
			str, ok := src.(string)
			if !ok {
				log.Error().Msgf("[stream] NewStream: Expected string, got %v", src)
				continue
			}
			s.producers = append(s.producers, NewProducer(str))
		}
		return s
	case map[string]any:
		return NewStream(source["url"])
	case nil:
		return new(Stream)
	default:
		panic(core.Caller())
	}
}

func (s *Stream) Sources() []string {
	sources := make([]string, 0, len(s.producers))
	for _, prod := range s.producers {
		sources = append(sources, prod.url)
	}
	return sources
}

func (s *Stream) SetSource(source string) {
	for _, prod := range s.producers {
		prod.SetSource(source)
	}
}

func (s *Stream) RemoveConsumer(cons core.Consumer) {
	_ = cons.Stop()

	s.mu.Lock()
	for i, consumer := range s.consumers {
		if consumer == cons {
			s.consumers = append(s.consumers[:i], s.consumers[i+1:]...)
			break
		}
	}
	s.mu.Unlock()

	s.stopProducers()
}

func (s *Stream) AddProducer(prod core.Producer) {
	producer := &Producer{conn: prod, state: stateExternal, url: "external"}
	s.mu.Lock()
	s.producers = append(s.producers, producer)
	s.mu.Unlock()
}

func (s *Stream) RemoveProducer(prod core.Producer) {
	s.mu.Lock()
	for i, producer := range s.producers {
		if producer.conn == prod {
			s.producers = append(s.producers[:i], s.producers[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
}

func (s *Stream) stopProducers() {
	if s.pending.Load() > 0 {
		log.Trace().Msg("[streams] skip stop pending producer")
		return
	}

	s.mu.Lock()
producers:
	for _, producer := range s.producers {
		for _, track := range producer.receivers {
			if len(track.Senders()) > 0 {
				continue producers
			}
		}
		for _, track := range producer.senders {
			if len(track.Senders()) > 0 {
				continue producers
			}
		}
		producer.stop()
	}
	s.mu.Unlock()
}

func (s *Stream) MarshalJSON() ([]byte, error) {
	var info = struct {
		Producers []*Producer     `json:"producers"`
		Consumers []core.Consumer `json:"consumers"`
	}{
		Producers: s.producers,
		Consumers: s.consumers,
	}
	return json.Marshal(info)
}

// CameraCodec discovers the audio codec that the camera (first producer) uses.
// Returns nil if no audio codec is found.
func (s *Stream) CameraCodec() *core.Codec {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, prod := range s.producers {
		if prod.conn == nil {
			continue
		}
		for _, media := range prod.conn.GetMedias() {
			if media.Kind != core.KindAudio {
				continue
			}
			// Prefer recvonly (camera sending audio to us)
			if media.Direction != core.DirectionRecvonly {
				continue
			}
			for _, codec := range media.Codecs {
				// Skip generic/any codecs
				if codec.Name == core.CodecAny || codec.Name == core.CodecAll {
					continue
				}
				// Skip video codecs that somehow ended up in audio media
				if codec.IsVideo() {
					continue
				}
				return codec
			}
		}
	}
	return nil
}

// BestSIPCodec returns the best audio codec from all stream producers,
// preferring codecs that are most widely compatible with SIP.
// Priority: Opus > G722 > PCMA > PCMU > PCM > PCML
// Returns nil if no audio codec is found.
func (s *Stream) BestSIPCodec() *core.Codec {
	s.mu.Lock()
	defer s.mu.Unlock()

	var best *core.Codec
	var bestPriority int

	for _, prod := range s.producers {
		if prod.conn == nil {
			continue
		}
		for _, media := range prod.conn.GetMedias() {
			if media.Kind != core.KindAudio {
				continue
			}
			if media.Direction != core.DirectionRecvonly {
				continue
			}
			for _, codec := range media.Codecs {
				if codec.Name == core.CodecAny || codec.Name == core.CodecAll {
					continue
				}
				if codec.IsVideo() {
					continue
				}
				p := codecPriority(codec.Name)
				if p > 0 && (best == nil || p > bestPriority) {
					best = codec
					bestPriority = p
				}
			}
		}
	}
	return best
}

func codecPriority(name string) int {
	switch name {
	case core.CodecOpus:
		return 5
	case core.CodecG722:
		return 4
	case core.CodecPCMA:
		return 3
	case core.CodecPCMU:
		return 2
	case core.CodecPCM, core.CodecPCML:
		return 1
	default:
		return 0
	}
}
