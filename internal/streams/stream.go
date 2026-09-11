package streams

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Stream struct {
	producers []*Producer
	consumers []core.Consumer
	control   string
	mu        sync.Mutex
	pending   atomic.Int32
}

func NewStream(source any) *Stream {
	switch source := source.(type) {
	case string:
		s := &Stream{producers: []*Producer{NewProducer(source)}}
		s.linkProducers()
		return s
	case []string:
		s := new(Stream)
		for _, str := range source {
			s.producers = append(s.producers, NewProducer(str))
		}
		s.linkProducers()
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
		s.linkProducers()
		return s
	case map[string]any:
		stream := NewStream(source["url"])
		if control, ok := source["control"].(string); ok {
			stream.control = control
		}
		return stream
	case nil:
		return new(Stream)
	default:
		panic(core.Caller())
	}
}

// linkProducers sets each Producer's stream back-reference. Required for
// Producer.reconnect to be able to notify the parent Stream
// (kickConsumers) after a successful reconnect.
func (s *Stream) linkProducers() {
	for _, p := range s.producers {
		p.stream = s
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

func (s *Stream) Move(pan, tilt float64) error {
	if s.control != "" {
		control := Get(s.control)
		if control == nil {
			return errors.New("streams: control stream not found")
		}
		return control.Move(pan, tilt)
	}

	var lastErr error
	for _, prod := range s.producers {
		if err := prod.Dial(); err != nil {
			lastErr = err
			continue
		}
		if err := prod.Move(pan, tilt); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("streams: PTZ not supported")
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
	remaining := len(s.consumers)
	s.mu.Unlock()

	// Trace-only: the per-transport disconnect log (e.g. `[rtsp] disconnect`)
	// already records who left, but it doesn't show the stream's resulting
	// consumer count. Surfacing that here closes a gap during incident
	// reconstruction — without it, you cannot tell from logs alone whether
	// a producer.stop() that follows is the last-consumer teardown or one
	// of many simultaneous removals.
	log.Trace().Int("remaining", remaining).Msg("[streams] consumer removed")

	s.stopProducers()
}

// remoteAddrer is the interface implemented by consumers whose underlying
// transport has a known remote address. Most go2rtc consumer types embed
// *core.Connection, which provides GetRemoteAddr() via promoted method —
// so this assertion succeeds for RTSP, WebRTC, etc.
type remoteAddrer interface {
	GetRemoteAddr() string
}

// kickGracePeriod is how long producers are held alive after kickConsumers
// fires so kicked clients can reconnect without hitting a cold producer
// that has to spin back up.
//
// 2 minutes — long enough to cover the worst case observed in practice:
// UniFi Protect's RTSP retry back-off after a session that closed shortly
// after opening (which it interprets as a server-instability signal).
// Empirically UniFi waits ~60-90s in that mode before retrying. We add
// margin on top of the advertised `Session:timeout=60` for safety.
//
// Trade-off: a producer with no consumers for the full grace window keeps
// streaming from upstream (small ongoing bandwidth cost — ~50-200 kbps
// for a typical Nest stream). Worth it to avoid recording gaps that
// require manual intervention to clear.
//
// Exposed as a package variable so tests can shorten it.
var kickGracePeriod = 2 * time.Minute

// kickConsumers disconnects all consumers attached to this stream.
// Called after a Producer reconnect when downstream consumers may be
// holding stale codec parameters (SPS/PPS) from the previous source
// session. By calling Stop() on each consumer, their underlying
// transports (e.g. RTSP TCP connections) close, prompting them to
// reconnect and re-DESCRIBE with the new producer's SDP.
//
// Consumers are not removed from s.consumers here — the natural
// close-path in each transport (e.g. tcpHandler in internal/rtsp)
// calls RemoveConsumer when its handler loop exits, keeping the list
// in sync.
//
// To keep producers alive during the reconnect window, this method
// bumps s.pending for kickGracePeriod. While pending is non-zero,
// stopProducers() short-circuits — so the producers we just
// reconnected stay attached and ready for the kicked consumers'
// near-immediate re-DESCRIBE. Without this, the cascade is
// kick → consumers gone → producers stop (no senders) → consumer
// reconnects to a cold producer → cold-start latency → consumer
// timeout, back-off cycle.
//
// reason is logged for observability; pass something specific like
// "producer reconnect: <scheme>".
func (s *Stream) kickConsumers(reason string) {
	s.mu.Lock()
	// Snapshot consumers and producer URLs in one critical section so the
	// internal-consumer skip below is consistent with the current set of
	// producers.
	consumers := make([]core.Consumer, len(s.consumers))
	copy(consumers, s.consumers)
	producerURLs := make(map[string]struct{}, len(s.producers))
	for _, p := range s.producers {
		if p.url != "" {
			producerURLs[p.url] = struct{}{}
		}
	}
	s.mu.Unlock()

	if len(consumers) == 0 {
		return
	}

	// Trace dump of the comparison inputs (producer URLs the skip-logic
	// is matching against). Combined with the per-consumer "kick filter"
	// trace below, this gives a full picture of why each consumer was or
	// wasn't kicked. URLs are redacted (query string stripped) to avoid
	// leaking credentials in shared trace dumps.
	if log.Trace().Enabled() {
		urls := make([]string, 0, len(producerURLs))
		for u := range producerURLs {
			urls = append(urls, redactSourceURL(u))
		}
		log.Trace().Strs("producer_urls", urls).Msg("[streams] kick start")
	}

	// Filter out internal-loop consumers — i.e. consumers whose source URL
	// matches one of this stream's producers. These are the inner ends of
	// recursive ffmpeg-style pipelines (e.g. `ffmpeg:driveway` is a
	// producer whose subprocess reads `rtsp://localhost/driveway` and
	// registers as a consumer). Kicking such a consumer kills the very
	// producer that just reconnected, triggering an instant reconnect loop.
	// Matches the loop-protection check in AddConsumer.
	toKick := make([]core.Consumer, 0, len(consumers))
	remotes := make([]string, 0, len(consumers))
	skipped := make([]string, 0)
	for _, cons := range consumers {
		// Diagnostic: trace-level per-consumer match attempt. Helps verify
		// the skip-logic is correctly engaging (or diagnose why it's not)
		// without enabling per-line debug log dumps. Visible at
		// streams=trace.
		var consSource, consRemote string
		if info, ok := cons.(core.Info); ok {
			consSource = info.GetSource()
		}
		if r, ok := cons.(remoteAddrer); ok {
			consRemote = r.GetRemoteAddr()
		}

		_, isInternal := producerURLs[consSource]
		log.Trace().
			Str("remote", consRemote).
			Str("source", redactSourceURL(consSource)).
			Bool("is_internal", isInternal && consSource != "").
			Msg("[streams] kick filter")

		if isInternal && consSource != "" {
			skipped = append(skipped, consSource)
			continue
		}

		toKick = append(toKick, cons)
		if consRemote != "" {
			remotes = append(remotes, consRemote)
		}
	}

	if len(toKick) == 0 {
		// Every consumer was an internal-loop. Surface this at debug so
		// the no-op kick is visible during diagnosis — otherwise a
		// reconnect cycle dominated by internal consumers would show
		// repeated "producer reconnected" lines with no kick output and
		// look mysteriously inert.
		if len(skipped) > 0 {
			log.Debug().
				Int("consumers", len(consumers)).
				Strs("skipped_internal", skipped).
				Str("reason", reason).
				Msg("[streams] kick skipped: all consumers are internal-loop")
		}
		return
	}

	ev := log.Debug().
		Int("count", len(toKick)).
		Strs("remotes", remotes).
		Str("reason", reason)
	if len(skipped) > 0 {
		// Tells operators which producers' inner consumers were exempted,
		// which both explains lower kick counts and surfaces topology.
		ev = ev.Strs("skipped_internal", skipped)
	}
	ev.Msg("[streams] kicking consumers")

	// Hold producers alive during the grace window. See kickGracePeriod.
	s.pending.Add(1)
	time.AfterFunc(kickGracePeriod, func() {
		if s.pending.Add(-1) == 0 {
			s.stopProducers()
		}
	})

	for _, cons := range toKick {
		_ = cons.Stop()
	}
}

func (s *Stream) AddProducer(prod core.Producer) {
	producer := &Producer{conn: prod, state: stateExternal, url: "external", stream: s}
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
	var stopped, kept int
producers:
	for _, producer := range s.producers {
		for _, track := range producer.receivers {
			if len(track.Senders()) > 0 {
				kept++
				continue producers
			}
		}
		for _, track := range producer.senders {
			if len(track.Senders()) > 0 {
				kept++
				continue producers
			}
		}
		producer.stop()
		stopped++
	}
	s.mu.Unlock()

	// Per-call summary: which branch did the decision take? Until now,
	// stopProducers either logged "skip stop pending producer" or was
	// silent — so a stopProducers that kept all producers (senders still
	// attached) looked identical in logs to one that wasn't called at
	// all. With this line we can tell when producers WERE kept alive
	// because senders held them up vs. when no consumer left in the
	// first place.
	if stopped > 0 || kept > 0 {
		log.Trace().Int("stopped", stopped).Int("kept", kept).Msg("[streams] stopProducers")
	}
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
