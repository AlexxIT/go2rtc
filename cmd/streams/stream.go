package streams

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
)

type Consumer struct {
	element streamer.Consumer
	tracks  []*streamer.Track
}

type Stream struct {
	producers []*Producer
	consumers []*Consumer
}

func NewStream(source interface{}) *Stream {
	s := new(Stream)

	switch source := source.(type) {
	case string:
		prod := &Producer{url: source}
		s.producers = append(s.producers, prod)
	case []interface{}:
		for _, source := range source {
			prod := &Producer{url: source.(string)}
			s.producers = append(s.producers, prod)
		}
	case map[string]interface{}:
		return NewStream(source["url"])
	default:
		panic("wrong source type")
	}

	return s
}

func (s *Stream) AddConsumer(cons streamer.Consumer) (err error) {
	ic := len(s.consumers)

	consumer := &Consumer{element: cons}

	// Step 1. Get consumer medias
	for icc, consMedia := range cons.GetMedias() {
		log.Trace().Stringer("media", consMedia).
			Msgf("[streams] consumer:%d:%d candidate", ic, icc)

	producers:
		for ip, prod := range s.producers {
			// Step 2. Get producer medias (not tracks yet)
			for ipc, prodMedia := range prod.GetMedias() {
				log.Trace().Stringer("media", prodMedia).
					Msgf("[streams] producer:%d:%d candidate", ip, ipc)

				// Step 3. Match consumer/producer codecs list
				prodCodec := prodMedia.MatchMedia(consMedia)
				if prodCodec != nil {
					log.Trace().Stringer("codec", prodCodec).
						Msgf("[streams] match producer:%d:%d => consumer:%d:%d", ip, ipc, ic, icc)

					// Step 4. Get producer track
					prodTrack := prod.GetTrack(prodMedia, prodCodec)

					// Step 5. Add track to consumer and get new track
					consTrack := consumer.element.AddTrack(consMedia, prodTrack)

					consumer.tracks = append(consumer.tracks, consTrack)
					break producers
				}
			}
		}
	}

	// can't match tracks for consumer
	if len(consumer.tracks) == 0 {
		return nil
	}

	s.consumers = append(s.consumers, consumer)

	for _, prod := range s.producers {
		prod.start()
	}

	return nil
}

func (s *Stream) RemoveConsumer(cons streamer.Consumer) {
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

	for _, producer := range s.producers {
		var sink bool
		for _, track := range producer.tracks {
			if len(track.Sink) > 0 {
				sink = true
			}
		}
		if !sink {
			producer.stop()
		}
	}
}

func (s *Stream) AddProducer(prod streamer.Producer) {
	panic("not implemented")
}

func (s *Stream) RemoveProducer(prod streamer.Producer) {
	panic("not implemented")
}

func (s *Stream) Active() bool {
	if len(s.consumers) > 0{
		return true
	}

	for _, prod := range s.producers {
		if prod.element != nil {
			return true
		}
	}

	return false
}

func (s *Stream) MarshalJSON() ([]byte, error) {
	var v []interface{}
	for _, prod := range s.producers {
		if prod.element != nil {
			v = append(v, prod.element)
		}
	}
	for _, cons := range s.consumers {
		// cons.element always not nil
		v = append(v, cons.element)
	}
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
