package streams

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"strings"
	"sync"
)

type Consumer struct {
	element streamer.Consumer
	tracks  []*streamer.Track
}

type Stream struct {
	producers []*Producer
	consumers []*Consumer
	mu        sync.Mutex
}

func NewStream(source interface{}) *Stream {
	switch source := source.(type) {
	case string:
		s := new(Stream)
		prod := &Producer{url: source}
		s.producers = append(s.producers, prod)
		return s
	case []interface{}:
		s := new(Stream)
		for _, source := range source {
			prod := &Producer{url: source.(string)}
			s.producers = append(s.producers, prod)
		}
		return s
	case *Stream:
		return source
	case map[string]interface{}:
		return NewStream(source["url"])
	case nil:
		return new(Stream)
	default:
		panic("wrong source type")
	}
}

func (s *Stream) SetSource(source string) {
	for _, prod := range s.producers {
		prod.SetSource(source)
	}
}

func (s *Stream) AddConsumer(cons streamer.Consumer) (err error) {
	ic := len(s.consumers)

	consumer := &Consumer{element: cons}
	var producers []*Producer // matched producers for consumer

	var codecs string

	// Step 1. Get consumer medias
	for icc, consMedia := range cons.GetMedias() {
		log.Trace().Stringer("media", consMedia).
			Msgf("[streams] consumer=%d candidate=%d", ic, icc)

	producers:
		for ip, prod := range s.producers {
			// Step 2. Get producer medias (not tracks yet)
			for ipc, prodMedia := range prod.GetMedias() {
				log.Trace().Stringer("media", prodMedia).
					Msgf("[streams] producer=%d candidate=%d", ip, ipc)

				collectCodecs(prodMedia, &codecs)

				// Step 3. Match consumer/producer codecs list
				prodCodec := prodMedia.MatchMedia(consMedia)
				if prodCodec != nil {
					log.Trace().Stringer("codec", prodCodec).
						Msgf("[streams] match producer:%d:%d => consumer:%d:%d", ip, ipc, ic, icc)

					// Step 4. Get producer track
					prodTrack := prod.GetTrack(prodMedia, prodCodec)
					if prodTrack == nil {
						log.Warn().Msg("[stream] can't get track")
						continue
					}

					// Step 5. Add track to consumer and get new track
					consTrack := consumer.element.AddTrack(consMedia, prodTrack)

					consumer.tracks = append(consumer.tracks, consTrack)
					producers = append(producers, prod)
					break producers
				}
			}
		}
	}

	if len(producers) == 0 {
		s.stopProducers()

		if len(codecs) > 0 {
			return errors.New("codecs not match: " + codecs)
		}

		for i, producer := range s.producers {
			if producer.lastErr != nil {
				return fmt.Errorf("source %d error: %w", i, producer.lastErr)
			}
		}

		return fmt.Errorf("sources unavailable: %d", len(s.producers))
	}

	s.mu.Lock()
	s.consumers = append(s.consumers, consumer)
	s.mu.Unlock()

	// there may be duplicates, but that's not a problem
	for _, prod := range producers {
		prod.start()
	}

	return nil
}

func (s *Stream) RemoveConsumer(cons streamer.Consumer) {
	s.mu.Lock()
	for i, consumer := range s.consumers {
		if consumer.element == cons {
			// remove consumer pads from all producers
			for _, track := range consumer.tracks {
				track.Unbind()
			}
			// remove consumer from slice
			s.removeConsumer(i)
			break
		}
	}
	s.mu.Unlock()

	s.stopProducers()
}

func (s *Stream) AddProducer(prod streamer.Producer) {
	producer := &Producer{element: prod, state: stateExternal}
	s.mu.Lock()
	s.producers = append(s.producers, producer)
	s.mu.Unlock()
}

func (s *Stream) RemoveProducer(prod streamer.Producer) {
	s.mu.Lock()
	for i, producer := range s.producers {
		if producer.element == prod {
			s.removeProducer(i)
			break
		}
	}
	s.mu.Unlock()
}

func (s *Stream) stopProducers() {
	s.mu.Lock()
producers:
	for _, producer := range s.producers {
		for _, track := range producer.tracks {
			if track.HasSink() {
				continue producers
			}
		}
		producer.stop()
	}
	s.mu.Unlock()
}

//func (s *Stream) Active() bool {
//	if len(s.consumers) > 0 {
//		return true
//	}
//
//	for _, prod := range s.producers {
//		if prod.element != nil {
//			return true
//		}
//	}
//
//	return false
//}

func (s *Stream) MarshalJSON() ([]byte, error) {
	var v []interface{}
	s.mu.Lock()
	for _, prod := range s.producers {
		if prod.element != nil {
			v = append(v, prod.element)
		}
	}
	for _, cons := range s.consumers {
		// cons.element always not nil
		v = append(v, cons.element)
	}
	s.mu.Unlock()
	if len(v) == 0 {
		v = nil
	}
	return json.Marshal(v)
}

func (s *Stream) removeConsumer(i int) {
	switch {
	case len(s.consumers) == 1: // only one element
		s.consumers = nil
	case i == 0: // first element
		s.consumers = s.consumers[1:]
	case i == len(s.consumers)-1: // last element
		s.consumers = s.consumers[:i]
	default: // middle element
		s.consumers = append(s.consumers[:i], s.consumers[i+1:]...)
	}
}

func (s *Stream) removeProducer(i int) {
	switch {
	case len(s.producers) == 1: // only one element
		s.producers = nil
	case i == 0: // first element
		s.producers = s.producers[1:]
	case i == len(s.producers)-1: // last element
		s.producers = s.producers[:i]
	default: // middle element
		s.producers = append(s.producers[:i], s.producers[i+1:]...)
	}
}

func collectCodecs(media *streamer.Media, codecs *string) {
	for _, codec := range media.Codecs {
		name := codec.Name
		if name == streamer.CodecAAC {
			name = "AAC"
		}
		if strings.Contains(*codecs, name) {
			continue
		}
		if len(*codecs) > 0 {
			*codecs += ","
		}
		*codecs += name
	}
}
