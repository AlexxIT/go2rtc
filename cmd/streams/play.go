package streams

import (
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
)

func (s *Stream) Play(source string) error {
	s.mu.Lock()
	for _, producer := range s.producers {
		if producer.state == stateInternal && producer.element != nil {
			_ = producer.element.Stop()
		}
	}
	s.mu.Unlock()

	if source == "" {
		return nil
	}

	var src streamer.Producer

	for _, producer := range s.producers {
		// start new client
		dst, err := GetProducer(producer.url)
		if err != nil {
			continue
		}

		// check if client support consumer interface
		cons, ok := dst.(streamer.Consumer)
		if !ok {
			_ = dst.Stop()
			continue
		}

		// start new producer
		if src == nil {
			if src, err = GetProducer(source); err != nil {
				return err
			}
		}

		if !matchMedia(src, cons) {
			_ = dst.Stop()
			continue
		}

		s.AddInternalProducer(src)
		s.AddInternalConsumer(cons)

		go func() {
			_ = src.Start()
			_ = dst.Stop()
			s.RemoveProducer(src)
		}()

		go func() {
			_ = dst.Start()
			_ = src.Stop()
			s.RemoveInternalConsumer(cons)
		}()

		return nil
	}

	return errors.New("can't find consumer")
}

func (s *Stream) AddInternalProducer(prod streamer.Producer) {
	producer := &Producer{element: prod, state: stateInternal}
	s.mu.Lock()
	s.producers = append(s.producers, producer)
	s.mu.Unlock()
}

func (s *Stream) AddInternalConsumer(cons streamer.Consumer) {
	consumer := &Consumer{element: cons}
	s.mu.Lock()
	s.consumers = append(s.consumers, consumer)
	s.mu.Unlock()
}

func (s *Stream) RemoveInternalConsumer(cons streamer.Consumer) {
	s.mu.Lock()
	for i, consumer := range s.consumers {
		if consumer.element == cons {
			s.removeConsumer(i)
			break
		}
	}
	s.mu.Unlock()
}

func matchMedia(prod streamer.Producer, cons streamer.Consumer) bool {
	for _, consMedia := range cons.GetMedias() {
		for _, prodMedia := range prod.GetMedias() {
			// codec negotiation
			prodCodec := prodMedia.MatchMedia(consMedia)
			if prodCodec == nil {
				continue
			}

			// setup producer track
			prodTrack := prod.GetTrack(prodMedia, prodCodec)
			if prodTrack == nil {
				return false
			}

			// setup consumer track
			consTrack := cons.AddTrack(consMedia, prodTrack)
			if consTrack == nil {
				return false
			}

			return true
		}
	}

	return false
}
