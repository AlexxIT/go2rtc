package streams

import (
	"encoding/json"
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"strings"
	"sync"
	"sync/atomic"
)

type Stream struct {
	producers []*Producer
	consumers []core.Consumer
	mu        sync.Mutex
	requests  int32
}

func NewStream(source any) *Stream {
	switch source := source.(type) {
	case string:
		s := new(Stream)
		prod := &Producer{url: source}
		s.producers = append(s.producers, prod)
		return s
	case []any:
		s := new(Stream)
		for _, source := range source {
			prod := &Producer{url: source.(string)}
			s.producers = append(s.producers, prod)
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

func (s *Stream) SetSource(source string) {
	for _, prod := range s.producers {
		prod.SetSource(source)
	}
}

func (s *Stream) AddConsumer(cons core.Consumer) (err error) {
	// support for multiple simultaneous requests from different consumers
	consN := atomic.AddInt32(&s.requests, 1) - 1

	var statErrors []error
	var statMedias []*core.Media
	var statProds []*Producer // matched producers for consumer

	// Step 1. Get consumer medias
	for _, consMedia := range cons.GetMedias() {
		log.Trace().Msgf("[streams] check cons=%d media=%s", consN, consMedia)

	producers:
		for prodN, prod := range s.producers {
			if err = prod.Dial(); err != nil {
				log.Trace().Err(err).Msgf("[streams] skip prod=%s", prod.url)
				statErrors = append(statErrors, err)
				continue
			}

			// Step 2. Get producer medias (not tracks yet)
			for _, prodMedia := range prod.GetMedias() {
				log.Trace().Msgf("[streams] check prod=%d media=%s", prodN, prodMedia)
				statMedias = append(statMedias, prodMedia)

				// Step 3. Match consumer/producer codecs list
				prodCodec, consCodec := prodMedia.MatchMedia(consMedia)
				if prodCodec == nil {
					continue
				}

				var track *core.Receiver

				switch prodMedia.Direction {
				case core.DirectionRecvonly:
					log.Trace().Msgf("[streams] match prod=%d => cons=%d", prodN, consN)

					// Step 4. Get recvonly track from producer
					if track, err = prod.GetTrack(prodMedia, prodCodec); err != nil {
						log.Info().Err(err).Msg("[streams] can't get track")
						continue
					}
					// Step 5. Add track to consumer
					if err = cons.AddTrack(consMedia, consCodec, track); err != nil {
						log.Info().Err(err).Msg("[streams] can't add track")
						continue
					}

				case core.DirectionSendonly:
					log.Trace().Msgf("[streams] match cons=%d => prod=%d", consN, prodN)

					// Step 4. Get recvonly track from consumer (backchannel)
					if track, err = cons.(core.Producer).GetTrack(consMedia, consCodec); err != nil {
						log.Info().Err(err).Msg("[streams] can't get track")
						continue
					}
					// Step 5. Add track to producer
					if err = prod.AddTrack(prodMedia, prodCodec, track); err != nil {
						log.Info().Err(err).Msg("[streams] can't add track")
						continue
					}
				}

				statProds = append(statProds, prod)

				if !consMedia.MatchAll() {
					break producers
				}
			}
		}
	}

	// stop producers if they don't have readers
	if atomic.AddInt32(&s.requests, -1) == 0 {
		s.stopProducers()
	}

	if len(statProds) == 0 {
		return formatError(statMedias, statErrors)
	}

	s.mu.Lock()
	s.consumers = append(s.consumers, cons)
	s.mu.Unlock()

	// there may be duplicates, but that's not a problem
	for _, prod := range statProds {
		prod.start()
	}

	return nil
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
	producer := &Producer{conn: prod, state: stateExternal}
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
	if !s.mu.TryLock() {
		log.Warn().Msgf("[streams] json locked")
		return json.Marshal(nil)
	}

	var info struct {
		Producers []*Producer     `json:"producers"`
		Consumers []core.Consumer `json:"consumers"`
	}
	info.Producers = s.producers
	info.Consumers = s.consumers

	s.mu.Unlock()

	return json.Marshal(info)
}

func formatError(statMedias []*core.Media, statErrors []error) error {
	var text string

	for _, media := range statMedias {
		if media.Direction == core.DirectionRecvonly {
			continue
		}

		for _, codec := range media.Codecs {
			name := codec.Name
			if name == core.CodecAAC {
				name = "AAC"
			}
			if strings.Contains(text, name) {
				continue
			}
			if len(text) > 0 {
				text += ","
			}
			text += name
		}
	}

	if text != "" {
		return errors.New(text)
	}

	for _, err := range statErrors {
		s := err.Error()
		if strings.Contains(text, s) {
			continue
		}
		if len(text) > 0 {
			text += ","
		}
		text += s
	}

	if text != "" {
		return errors.New(text)
	}

	return errors.New("unknown error")
}
