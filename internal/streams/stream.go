package streams

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Stream struct {
	producers []*Producer
	consumers []core.Consumer
	mu        sync.Mutex
	pending   atomic.Int32

	// Track connected consumer-track pairs to prevent duplicates
	connectionsMu sync.Mutex
	connections   map[connectionKey]bool
}

type connectionKey struct {
	consumerPtr uintptr
	trackPtr    uintptr
}

func NewStream(source any) *Stream {
	switch source := source.(type) {
	case string:
		return &Stream{
			producers:   []*Producer{NewProducer(source)},
			connections: make(map[connectionKey]bool),
		}
	case []string:
		s := new(Stream)
		s.connections = make(map[connectionKey]bool)
		for _, str := range source {
			s.producers = append(s.producers, NewProducer(str))
		}
		return s
	case []any:
		s := new(Stream)
		s.connections = make(map[connectionKey]bool)
		for _, src := range source {
			str, ok := src.(string)
			if !ok {
				log.Error().Msgf("[stream] NewStream: Expected string, got %v", src)
				continue
			}
			prod := NewProducer(str)
			s.producers = append(s.producers, prod)
		}
		return s
	case map[string]any:
		return NewStream(source["url"])
	case nil:
		s := new(Stream)
		s.connections = make(map[connectionKey]bool)
		return s
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

	// Check if removed consumer was external (not FFmpeg/internal)
	removedWasExternal := true
	consSource := ""
	if info, ok := cons.(core.Info); ok {
		consSource = info.GetSource()
		if consSource == "" || strings.HasPrefix(consSource, "ffmpeg:") {
			removedWasExternal = false
		}
	}
	log.Debug().Msgf("[streams] RemoveConsumer source=%s external=%v", consSource, removedWasExternal)

	s.mu.Lock()
	for i, consumer := range s.consumers {
		if consumer == cons {
			s.consumers = append(s.consumers[:i], s.consumers[i+1:]...)
			break
		}
	}

	// Only run aggressive cleanup if an external consumer was removed
	if removedWasExternal {
		// Check if there are any non-FFmpeg consumers remaining
		hasExternalConsumers := false
		for _, consumer := range s.consumers {
			// Check if this consumer is an external consumer (not FFmpeg transcoder, not HomeKit speaker)
			if info, ok := consumer.(core.Info); ok {
				source := info.GetSource()
				// Internal consumers:
				// - FFmpeg transcoder consumers have sources like "ffmpeg:aqara_g4_webrtc#..."
				// - Empty source is internal RTSP consumer
				// - HomeKit speaker consumers have sources like "homekit://..."
				isInternal := source == "" || strings.HasPrefix(source, "ffmpeg:") || strings.HasPrefix(source, "homekit://")
				if !isInternal {
					hasExternalConsumers = true
					break
				}
			} else {
				// If we can't determine the source, assume it's external to be safe
				hasExternalConsumers = true
				break
			}
		}

		// If no external consumers remain, remove all internal consumers
		if !hasExternalConsumers {
			log.Debug().Msg("[streams] no external consumers, running cleanup")
			var externalConsumers []core.Consumer
			for _, consumer := range s.consumers {
				isInternalConsumer := false
				if info, ok := consumer.(core.Info); ok {
					source := info.GetSource()
					// Internal consumers: FFmpeg, empty source (RTSP), HomeKit speaker
					if source == "" || strings.HasPrefix(source, "ffmpeg:") || strings.HasPrefix(source, "homekit://") {
						isInternalConsumer = true
						_ = consumer.Stop()
					}
				}
				if !isInternalConsumer {
					externalConsumers = append(externalConsumers, consumer)
				}
			}
			s.consumers = externalConsumers

			// Also stop all FFmpeg/exec/HomeKit producers
			for _, producer := range s.producers {
				if strings.HasPrefix(producer.url, "ffmpeg:") || strings.HasPrefix(producer.url, "exec:") || strings.HasPrefix(producer.url, "homekit://") {
					producer.stop()
				}
			}
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

func (s *Stream) RegisterHomekitSpeakers() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, producer := range s.producers {
		// Check if this is a HomeKit producer
		if !strings.HasPrefix(producer.url, "homekit://") {
			continue
		}

		// Check if it's already registered as consumer
		alreadyRegistered := false
		for _, consumer := range s.consumers {
			if consInfo, ok := consumer.(core.Info); ok && producer.conn != nil {
				if prodInfo, ok := producer.conn.(core.Info); ok {
					if consInfo.GetSource() == prodInfo.GetSource() {
						alreadyRegistered = true
						break
					}
				}
			}
		}

		if alreadyRegistered {
			continue
		}

		// Check if this HomeKit has speaker capability
		if producer.conn != nil {
			medias := producer.conn.GetMedias()
			for _, media := range medias {
				if media.Kind == core.KindAudio && media.Direction == core.DirectionSendonly {
					// This HomeKit has speaker capability - add it as consumer
					if consumer, ok := producer.conn.(core.Consumer); ok {
						s.consumers = append(s.consumers, consumer)
					}
					break
				}
			}
		}
	}
}

func (s *Stream) MatchConsumersWithProducer(prod core.Producer) {
	s.mu.Lock()
	existingConsumers := make([]core.Consumer, len(s.consumers))
	copy(existingConsumers, s.consumers)
	existingProducers := make([]*Producer, len(s.producers))
	copy(existingProducers, s.producers)
	s.mu.Unlock()

	// First match each existing consumer with the new producer
	for _, consumer := range existingConsumers {
		consumerMedias := consumer.GetMedias()
		producerMedias := prod.GetMedias()

		for _, consMedia := range consumerMedias {
			for _, prodMedia := range producerMedias {
				prodCodec, consCodec := prodMedia.MatchMedia(consMedia)
				if prodCodec == nil {
					continue
				}

				var track *core.Receiver
				var err error

				if prodMedia.Direction == core.DirectionRecvonly {
					// Producer provides media to consumer
					if track, err = prod.GetTrack(prodMedia, prodCodec); err != nil {
						continue
					}

					// Check if this consumer-track combination already exists to prevent duplicates
					key := connectionKey{
						consumerPtr: uintptr(reflect.ValueOf(consumer).UnsafePointer()),
						trackPtr:    uintptr(unsafe.Pointer(track)),
					}

					s.connectionsMu.Lock()
					if s.connections[key] {
						s.connectionsMu.Unlock()
						continue
					}

					if err = consumer.AddTrack(consMedia, consCodec, track); err != nil {
						s.connectionsMu.Unlock()
						continue
					}

					// Mark this connection as established to prevent future duplicates
					s.connections[key] = true
					s.connectionsMu.Unlock()

					// Start the producer
					s.mu.Lock()
					for _, producer := range s.producers {
						if producer.conn == prod {
							producer.start()
							break
						}
					}
					s.mu.Unlock()
				}
			}
		}
	}

	// After checking regular consumers, check existing producers that can act as consumers (e.g., HomeKit speaker)
	for _, existingProducer := range existingProducers {
		if existingProducer.conn == nil {
			continue // Skip producers that aren't connected yet
		}

		// Check if the existing producer can act as a consumer
		if existingConsumer, ok := existingProducer.conn.(core.Consumer); ok {
			consumerMedias := existingConsumer.GetMedias()
			producerMedias := prod.GetMedias()

			for _, consMedia := range consumerMedias {
				for _, prodMedia := range producerMedias {
					prodCodec, consCodec := prodMedia.MatchMedia(consMedia)
					if prodCodec == nil {
						continue
					}

					var track *core.Receiver
					var err error

					if prodMedia.Direction == core.DirectionRecvonly {
						// New producer provides media to existing producer (acting as consumer)
						if track, err = prod.GetTrack(prodMedia, prodCodec); err != nil {
							continue
						}

						// Check for duplicate connections
						key := connectionKey{
							consumerPtr: uintptr(reflect.ValueOf(existingConsumer).UnsafePointer()),
							trackPtr:    uintptr(unsafe.Pointer(track)),
						}

						s.connectionsMu.Lock()
						if s.connections[key] {
							s.connectionsMu.Unlock()
							continue
						}

						if err = existingConsumer.AddTrack(consMedia, consCodec, track); err != nil {
							s.connectionsMu.Unlock()
							continue
						}

						// Mark this connection as established to prevent future duplicates
						s.connections[key] = true
						s.connectionsMu.Unlock()

						// Start the producer after successful connection
						s.mu.Lock()
						for _, producer := range s.producers {
							if producer.conn == prod {
								producer.start()
								break
							}
						}
						s.mu.Unlock()
					}
				}
			}
		}
	}
}

func (s *Stream) UpdateFFmpegProducerConnection(rtspConn core.Producer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, producer := range s.producers {
		// Look for FFmpeg producer with accept-audio
		if strings.HasPrefix(producer.url, "ffmpeg:") && strings.Contains(producer.url, "accept-audio") {
			// Update the producer connection
			producer.conn = rtspConn

			// Set the producer state to stateTracks since FFmpeg producer already has tracks available
			producer.state = stateTracks

			// Clear existing connection tracking entries for FFmpeg producers to allow reconnection with new tracks
			s.connectionsMu.Lock()
			var keysToDelete []connectionKey
			for key := range s.connections {
				// Find connections involving the updated producer
				if uintptr(reflect.ValueOf(rtspConn).UnsafePointer()) == key.consumerPtr {
					keysToDelete = append(keysToDelete, key)
				}
			}
			for _, key := range keysToDelete {
				delete(s.connections, key)
			}
			s.connectionsMu.Unlock()

			return
		}
	}
}

// TriggerReMatching triggers re-matching for all active producers with new consumer
// This is useful when a new consumer (like RTSP) connects and we need to check
// if any existing producers (like WebRTC) can now be connected to it
func (s *Stream) TriggerReMatching() {
	// Collect active producers without holding the mutex to avoid recursive locking
	var activeProducers []core.Producer

	s.mu.Lock()
	for _, producer := range s.producers {
		if producer.conn != nil && producer.state >= stateTracks {
			activeProducers = append(activeProducers, producer.conn)
		}
	}
	s.mu.Unlock()

	// Now call MatchConsumersWithProducer without holding the mutex
	for _, prod := range activeProducers {
		s.MatchConsumersWithProducer(prod)
	}
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

	// Check if any producers should be stopped now that this one is gone
	s.stopProducers()
}

func (s *Stream) stopProducers() {
	if s.pending.Load() > 0 {
		log.Trace().Msg("[streams] skip stop pending producer")
		return
	}

	s.mu.Lock()
producers:
	for _, producer := range s.producers {
		// Skip producers that haven't started yet (no tracks)
		if len(producer.receivers) == 0 && len(producer.senders) == 0 {
			continue producers
		}

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
