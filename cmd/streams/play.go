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

	for _, producer := range s.producers {
		// start new client
		client, err := GetProducer(producer.url)
		if err != nil {
			return err
		}

		// check if client support consumer interface
		cons := client.(streamer.Consumer)
		if cons == nil {
			continue
		}

		// start new producer
		prod, err := GetProducer(source)
		if err != nil {
			return err
		}

		if !matchMedia(prod, cons) {
			return errors.New("can't match media")
		}

		s.AddInternalProducer(prod)
		s.AddInternalConsumer(cons)

		go func() {
			_ = prod.Start()
			_ = client.Stop()
			s.RemoveProducer(prod)
		}()

		go func() {
			_ = client.Start()
			_ = prod.Stop()
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
