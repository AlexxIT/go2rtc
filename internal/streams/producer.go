package streams

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// kickOnReconnect controls whether downstream consumers are forcibly
// disconnected after a Producer reconnects. Default: disabled.
//
// Background: the kick was added to refresh stale codec parameters
// (SPS/PPS) on RTSP clients like UniFi Protect after a WebRTC
// re-establishment produced new SDP. Empirically, UniFi Protect
// versions through ≤ 7.0.x re-DESCRIBE within 1-3s of an abrupt
// server-initiated TCP close, so the kick was a successful repair.
//
// UniFi Protect 7.1.69 changed behavior: after a server-initiated
// close, the recording session does NOT re-establish (observed:
// zero RTSP DESCRIBE attempts in the entire 2-minute grace window
// even though the snapshot poll continued to succeed against the
// same server). For those clients the kick is now actively
// harmful — it permanently severs the recording session that the
// reconnect was meant to refresh.
//
// Default-off because modern decoders (UniFi's GStreamer 1.26+
// included) generally handle in-band SPS/PPS changes via RTP, so
// the kick is unnecessary for them.
//
// Set GO2RTC_KICK_ON_RECONNECT=true to re-enable the legacy
// behavior — appropriate for older UniFi versions or other RTSP
// clients that genuinely cannot handle in-band parameter changes.
var kickOnReconnect = os.Getenv("GO2RTC_KICK_ON_RECONNECT") == "true"

// redactSourceURL returns a producer URL with the query string stripped,
// suitable for log output. Source URLs like `nest:?client_id=…&client_secret=…`
// or `rtsp://user:pass@host/stream?…` would otherwise leak credentials into
// log output that may be shared during debugging.
func redactSourceURL(rawURL string) string {
	if i := strings.IndexAny(rawURL, "?#"); i >= 0 {
		return rawURL[:i]
	}
	return rawURL
}

type state byte

const (
	stateNone state = iota
	stateMedias
	stateTracks
	stateStart
	stateExternal
	stateInternal
)

type Producer struct {
	core.Listener

	url      string
	template string

	conn      core.Producer
	receivers []*core.Receiver
	senders   []*core.Receiver

	state    state
	mu       sync.Mutex
	workerID int

	// stream is a back-reference to the parent Stream, set by NewStream /
	// AddProducer. nil for standalone producers created via NewProducer().
	// Used after a successful reconnect to notify the stream that downstream
	// consumers should disconnect — see Stream.kickConsumers.
	stream *Stream
}

const SourceTemplate = "{input}"

func NewProducer(source string) *Producer {
	if strings.Contains(source, SourceTemplate) {
		return &Producer{template: source}
	}

	return &Producer{url: source}
}

func (p *Producer) SetSource(s string) {
	if p.template == "" {
		p.url = s
	} else {
		p.url = strings.Replace(p.template, SourceTemplate, s, 1)
	}
}

func (p *Producer) Dial() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == stateNone {
		conn, err := GetProducer(p.url)
		if err != nil {
			return err
		}

		p.conn = conn
		p.state = stateMedias
	}

	return nil
}

func (p *Producer) GetMedias() []*core.Media {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn == nil {
		return nil
	}

	return p.conn.GetMedias()
}

func (p *Producer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == stateNone {
		return nil, errors.New("get track from none state")
	}

	for _, track := range p.receivers {
		if track.Codec == codec {
			return track, nil
		}
	}

	track, err := p.conn.GetTrack(media, codec)
	if err != nil {
		return nil, err
	}

	p.receivers = append(p.receivers, track)

	if p.state == stateMedias {
		p.state = stateTracks
	}

	return track, nil
}

func (p *Producer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == stateNone {
		return errors.New("add track from none state")
	}

	if err := p.conn.(core.Consumer).AddTrack(media, codec, track); err != nil {
		return err
	}

	p.senders = append(p.senders, track)

	if p.state == stateMedias {
		p.state = stateTracks
	}

	return nil
}

func (p *Producer) Move(pan, tilt float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == stateNone {
		return errors.New("move from none state")
	}

	ptz, ok := p.conn.(core.PTZ)
	if !ok {
		return errors.New("PTZ not supported")
	}

	return ptz.Move(pan, tilt)
}

func (p *Producer) MarshalJSON() ([]byte, error) {
	if conn := p.conn; conn != nil {
		return json.Marshal(conn)
	}
	info := map[string]string{"url": p.url}
	return json.Marshal(info)
}

// internals

func (p *Producer) start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != stateTracks {
		return
	}

	log.Debug().Msgf("[streams] start producer url=%s", redactSourceURL(p.url))

	p.state = stateStart
	p.workerID++

	// If the underlying producer declares a known max stream lifetime
	// (e.g. nest WebRTC, ~60 min cap), wire up the proactive-reconnect
	// callback so it can trigger a connection swap before the limit
	// silently kills the stream. Logged at debug so operators can
	// confirm the wiring is in place without waiting ~55 min for the
	// first fire — a silent type-assertion failure would otherwise
	// only surface as a re-emergence of the periodic gap.
	if hook, ok := p.conn.(core.LifetimeLimited); ok {
		hook.SetReconnectCallback(p.proactiveReconnect)
		log.Debug().Str("source", redactSourceURL(p.url)).
			Msg("[streams] proactive-reconnect hook registered (source declares max stream lifetime)")
	}

	go p.worker(p.conn, p.workerID)
	go p.activityTick(p.workerID)
}

// proactiveReconnect triggers a fresh GetProducer + track swap without
// waiting for the current connection to die. Invoked via the
// SetReconnectCallback hook by producers that know their upstream has
// a hard lifetime cap (currently: Nest WebRTC at ~60 min).
//
// The existing reconnect() flow is already overlap-friendly: it
// allocates a new conn, moves tracks via Receiver.Replace(), then
// stops the old conn. The only gap consumers see is the
// GetProducer setup time (~3 s for Nest), and only on the receiver
// tracks that haven't started flowing through the new conn yet.
// That's small enough that UniFi's session-keepalive sees no
// disruption.
//
// workerID bookkeeping: bump before reconnect so the old worker —
// which will return shortly after reconnect calls p.conn.Stop() on
// its conn — sees a workerID mismatch in its post-Start reconnect
// call and no-ops. Without this, the old worker would race the new
// one into reconnect() and produce a second fresh connection.
func (p *Producer) proactiveReconnect() {
	p.mu.Lock()
	if p.state != stateStart {
		p.mu.Unlock()
		log.Trace().Str("source", redactSourceURL(p.url)).
			Msg("[streams] skip proactive reconnect: producer not started")
		return
	}
	p.workerID++
	newID := p.workerID
	p.mu.Unlock()

	log.Info().Str("source", redactSourceURL(p.url)).
		Msg("[streams] proactive reconnect (lifetime-limited source)")

	// The activityTick goroutine started in start() is tied to the
	// previous workerID and will exit on its next tick when it sees the
	// bump. Spawn a fresh one with the new ID so heartbeats keep
	// flowing through the new connection's lifetime — without this we
	// lose visibility into the producer immediately after the refresh,
	// which is the worst possible moment to go blind.
	go p.activityTick(newID)

	p.reconnect(newID, 0)
}

// activityInterval is the heartbeat cadence for the producer activity
// log. Each tick emits one debug line per receiver with packets/bytes
// deltas — visible only when --log.level=debug. The point is to make
// silent-stall failure modes obvious: if the upstream connection looks
// healthy (no EOF, no read-deadline fire) but video frames have stopped,
// the heartbeat shows dpackets=0 on the affected track while audio
// continues to tick over. Without this, those stalls are invisible
// because routine RTP forwarding does not log per packet.
//
// Exposed as a package variable so tests can shorten it.
var activityInterval = 60 * time.Second

// activityTick periodically logs per-receiver packet and byte counts
// for this producer. Runs in its own goroutine; exits when the
// producer's workerID advances (start of next cycle or stop).
//
// Reconnect does not advance workerID, so the goroutine survives a
// reconnect. Receivers are swapped via Receiver.Replace() in
// p.reconnect(), which means after reconnect the slots in p.receivers
// point at fresh *core.Receiver values whose counters start at 0. The
// prev map is keyed by pointer, so a freshly-swapped receiver gets a
// zero baseline on its first tick after reconnect — yielding a delta
// equal to the new receiver's lifetime packet count. That's an
// acceptable boundary blip; subsequent ticks are accurate deltas.
func (p *Producer) activityTick(workerID int) {
	ticker := time.NewTicker(activityInterval)
	defer ticker.Stop()

	type counter struct{ bytes, packets int }
	prev := make(map[*core.Receiver]counter)

	for range ticker.C {
		p.mu.Lock()
		if p.workerID != workerID {
			p.mu.Unlock()
			return
		}
		// Snapshot under lock — reconnect may swap p.receivers.
		receivers := make([]*core.Receiver, len(p.receivers))
		copy(receivers, p.receivers)
		url := p.url
		p.mu.Unlock()

		if !log.Debug().Enabled() {
			continue
		}

		for i, r := range receivers {
			c := prev[r]
			bytesNow, packetsNow := r.Bytes, r.Packets
			prev[r] = counter{bytes: bytesNow, packets: packetsNow}

			codec := "unknown"
			if r.Codec != nil {
				codec = r.Codec.Name
			}
			log.Debug().
				Str("source", redactSourceURL(url)).
				Int("track", i).
				Str("codec", codec).
				Int("dpackets", packetsNow-c.packets).
				Int("dbytes", bytesNow-c.bytes).
				Int("total_packets", packetsNow).
				Msg("[streams] producer activity")
		}
	}
}

func (p *Producer) worker(conn core.Producer, workerID int) {
	if err := conn.Start(); err != nil {
		p.mu.Lock()
		closed := p.workerID != workerID
		p.mu.Unlock()

		if closed {
			return
		}

		log.Warn().Err(err).Str("url", redactSourceURL(p.url)).Caller().Send()
	}

	p.reconnect(workerID, 0)
}

func (p *Producer) reconnect(workerID, retry int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.workerID != workerID {
		log.Trace().Msgf("[streams] stop reconnect url=%s", redactSourceURL(p.url))
		return
	}

	// First retry of a cycle is operationally interesting — surface it at
	// info so operators running at log.level=info see source-outage events
	// without enabling debug. Retry 5 is the threshold at which we
	// escalate to warn: by then the backoff has stretched to 5s/retry, so
	// the source has been down for ~10s+ and isn't coming back quickly.
	// Per-retry detail (`retry=N to url=…`) stays at debug to avoid noise.
	switch retry {
	case 0:
		log.Info().Str("source", redactSourceURL(p.url)).
			Msg("[streams] producer reconnecting")
	case 5:
		log.Warn().Str("source", redactSourceURL(p.url)).Int("retry", retry).
			Msg("[streams] producer reconnect still failing")
	}
	log.Debug().Msgf("[streams] retry=%d to url=%s", retry, redactSourceURL(p.url))

	conn, err := GetProducer(p.url)
	if err != nil {
		log.Debug().Msgf("[streams] producer=%s", err)

		timeout := time.Minute
		if retry < 5 {
			timeout = time.Second
		} else if retry < 10 {
			timeout = time.Second * 5
		} else if retry < 20 {
			timeout = time.Second * 10
		}

		time.AfterFunc(timeout, func() {
			p.reconnect(workerID, retry+1)
		})
		return
	}

	for _, media := range conn.GetMedias() {
		switch media.Direction {
		case core.DirectionRecvonly:
			for i, receiver := range p.receivers {
				codec := media.MatchCodec(receiver.Codec)
				if codec == nil {
					continue
				}

				track, err := conn.GetTrack(media, codec)
				if err != nil {
					continue
				}

				receiver.Replace(track)
				p.receivers[i] = track
				break
			}

		case core.DirectionSendonly:
			for _, sender := range p.senders {
				codec := media.MatchCodec(sender.Codec)
				if codec == nil {
					continue
				}

				_ = conn.(core.Consumer).AddTrack(media, codec, sender)
			}
		}
	}

	// stop previous connection after moving tracks (fix ghost exec/ffmpeg)
	_ = p.conn.Stop()
	// swap connections
	p.conn = conn

	// Re-register the proactive-reconnect callback on the new conn so
	// the next lifetime cycle is also caught. Without this, only the
	// first connection (from start()) benefits from proactive refresh.
	if hook, ok := conn.(core.LifetimeLimited); ok {
		hook.SetReconnectCallback(p.proactiveReconnect)
		log.Debug().Str("source", redactSourceURL(p.url)).
			Msg("[streams] proactive-reconnect hook re-registered after swap")
	}

	// Recovery signal at info level so operators at log.level=info see the
	// outage was resolved. Pairs with the info line at retry=0.
	log.Info().Str("source", redactSourceURL(p.url)).
		Msg("[streams] producer reconnected")

	// Disconnect downstream consumers so they re-DESCRIBE and pick up the
	// new source's SDP. Gated by kickOnReconnect because UniFi Protect
	// 7.1.69+ does not re-DESCRIBE after a server-initiated close, so
	// the kick now severs the recording session it was meant to repair.
	// See the package-level comment on kickOnReconnect for the full
	// rationale. Default behavior: no kick.
	if p.stream != nil && kickOnReconnect {
		// Use the URL scheme + path only (drops query string) so source
		// URLs that carry credentials in the query (nest:?client_secret=…,
		// etc.) don't leak them into the log line.
		go p.stream.kickConsumers("producer reconnect: " + redactSourceURL(p.url))
	}

	go p.worker(conn, workerID)
}

func (p *Producer) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch p.state {
	case stateExternal:
		log.Trace().Msgf("[streams] skip stop external producer")
		return
	case stateNone:
		log.Trace().Msgf("[streams] skip stop none producer")
		return
	case stateStart:
		p.workerID++
	}

	log.Debug().Msgf("[streams] stop producer url=%s", redactSourceURL(p.url))

	if p.conn != nil {
		_ = p.conn.Stop()
		p.conn = nil
	}

	p.state = stateNone
	p.receivers = nil
	p.senders = nil
}
