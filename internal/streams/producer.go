package streams

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
)

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

	// dialing is non-nil while a Dial() is in flight; it is closed when that
	// dial finishes. Concurrent callers wait on the channel INSTEAD of on mu,
	// so a slow dial can't freeze the producer. dialErr carries the result to
	// those waiters so they don't each re-spawn the same failing source.
	// Guarded by mu.
	dialing chan struct{}
	dialErr error

	// epoch is bumped by every teardown (stop, forceReset). A dial that
	// completes after its epoch changed is discarded instead of resurrecting
	// a producer nobody asked for any more. Guarded by mu.
	epoch int

	// connFails counts consecutive instant Start() failures with the
	// rtsp.ErrStartFromConn signature; connFailLimit of them in a row means
	// the conn/receiver state is provably inconsistent and the producer gets
	// force-reset to stateNone. Guarded by mu.
	connFails int
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

// Dial creates the underlying core.Producer if the producer has none yet.
//
// GetProducer can block for a long time: an exec source spawns a child and
// then reads its first bytes, an RTSP source dials and sends DESCRIBE, an
// HTTP source may accept the connection and never send a byte. Calling it
// with p.mu held freezes the whole producer for that whole time, because
// every other consumer's Dial and GetTrack, and stop(), all wait on the same
// mutex. If the dial never returns, the freeze is permanent and only a
// process restart clears it. The observed goroutine chain is
// Dial -> GetProducer -> exec.handlePipe -> magic.Open, with later consumers
// parked on sync.Mutex.Lock inside Dial.
//
// The dial now runs with mu released. A single-flight channel makes
// concurrent consumers wait on the in-flight dial and share its error, so
// only one child is spawned, and dialTimeout bounds the dial so a handler
// that never returns cannot strand the producer.
func (p *Producer) Dial() error {
	p.mu.Lock()

	if p.state != stateNone {
		p.mu.Unlock()
		return nil
	}

	// Someone else is already dialing: wait on them, not on the mutex, so
	// stop() and GetTrack() stay responsive for everyone else.
	if ch := p.dialing; ch != nil {
		p.mu.Unlock()

		select {
		case <-ch: // always closed, on success, failure and abandon
		case <-time.After(dialTimeout): // defensive only
			return ErrDialTimeout
		}

		p.mu.Lock()
		defer p.mu.Unlock()
		if p.state != stateNone {
			return nil
		}
		return p.dialErr
	}

	ch := make(chan struct{})
	p.dialing = ch
	epoch := p.epoch
	p.mu.Unlock()

	conn, err := dialBounded(p.url)

	p.mu.Lock()
	if p.dialing == ch {
		p.dialing = nil
	}
	if err == nil {
		if p.state == stateNone && p.epoch == epoch {
			p.conn = conn
			p.state = stateMedias
		} else {
			// producer was rebuilt or torn down while we were dialing: drop our
			// now-redundant connection rather than clobbering the live one or
			// resurrecting a stopped producer with a live child process
			err = ErrDialSuperseded
			go func() { _ = conn.Stop() }()
		}
	}
	p.dialErr = err
	p.mu.Unlock()

	close(ch)

	return err
}

// dialBounded runs GetProducer with a dialTimeout backstop. A handler that
// never returns, such as an exec child that writes no first byte or an HTTP
// source that accepts the connection and then goes silent, must not block the
// caller forever. On timeout the pending dial is handed to a reaper that
// stops whatever it eventually produces, so abandoning it costs at most one
// orphaned connection instead of a producer that never recovers.
func dialBounded(url string) (core.Producer, error) {
	type result struct {
		conn core.Producer
		err  error
	}

	done := make(chan result, 1)
	go func() {
		conn, err := GetProducer(url)
		done <- result{conn, err}
	}()

	select {
	case res := <-done:
		return res.conn, res.err
	case <-time.After(dialTimeout):
		go func() {
			if res := <-done; res.err == nil {
				_ = res.conn.Stop()
			}
		}()

		log.Error().Str("url", url).Msgf(
			"[streams] producer dial exceeded %s, abandoning", dialTimeout,
		)

		return nil, ErrDialTimeout
	}
}

// dialTimeout is the producer-level backstop on GetProducer. It sits well
// above every handler's own bound, so it cannot cut short a dial that
// succeeds today; it only fires where the current behaviour is to block
// forever. That makes the guarantee source-agnostic: a handler that forgets
// its own timeout can no longer strand the producer. The cost of firing is
// one orphaned connection, which the reaper stops if it ever materialises.
// It is a var, not a const, only so tests can shrink it.
var dialTimeout = 90 * time.Second

var (
	// ErrDialSuperseded means a dial completed after the producer it was
	// dialing for had already been torn down or rebuilt. The connection is
	// discarded; the caller should just retry.
	ErrDialSuperseded = errors.New("streams: dial superseded")

	// ErrDialTimeout means GetProducer blew dialTimeout. The dial is abandoned
	// and consumers get an error instead of parking on the producer forever.
	ErrDialTimeout = errors.New("streams: dial timeout")
)

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

	log.Debug().Msgf("[streams] start producer url=%s", p.url)

	p.state = stateStart
	p.workerID++

	go p.worker(p.conn, p.workerID)
}

// connFailLimit is how many consecutive rtsp.ErrStartFromConn failures a
// producer gets before forceReset. Each cycle is near-instant: dial, error,
// then redial with retry=0 and so no backoff. Without a limit, a redialed
// connection whose medias no longer match the stale receivers turns into a
// hot error loop that fills the log with "start from CONN state".
const connFailLimit = 3

func (p *Producer) worker(conn core.Producer, workerID int) {
	err := conn.Start()

	p.mu.Lock()
	closed := p.workerID != workerID
	if !closed {
		if errors.Is(err, rtsp.ErrStartFromConn) {
			p.connFails++
		} else {
			p.connFails = 0
		}
	}
	fails := p.connFails
	p.mu.Unlock()

	if closed {
		return
	}

	if err != nil {
		log.Warn().Err(err).Str("url", p.url).Caller().Send()
	}

	if fails >= connFailLimit {
		p.forceReset(workerID)
		return
	}

	p.reconnect(workerID, 0)
}

// forceReset tears a stuck producer down to stateNone so the next consumer
// redials from scratch, instead of hot-looping Start() errors against a
// connection whose medias no longer match the receivers. It is only reached
// after connFailLimit consecutive rtsp.ErrStartFromConn failures. Any
// successful Start, or any other error, resets the counter, so a healthy or
// merely flaky producer never hits it.
func (p *Producer) forceReset(workerID int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.workerID != workerID {
		return // someone else already took over
	}

	switch p.state {
	case stateExternal, stateNone:
		return
	}

	log.Warn().Str("url", p.url).Msgf(
		"[streams] producer force-reset after %d consecutive CONN-state start failures",
		connFailLimit,
	)

	p.workerID++ // invalidate stale workers
	p.epoch++    // invalidate any dial still in flight

	if p.conn != nil {
		_ = p.conn.Stop()
		p.conn = nil
	}

	p.state = stateNone
	p.receivers = nil
	p.senders = nil
	p.connFails = 0
}

func (p *Producer) reconnect(workerID, retry int) {
	p.mu.Lock()
	if p.workerID != workerID {
		log.Trace().Msgf("[streams] stop reconnect url=%s", p.url)
		p.mu.Unlock()
		return
	}
	url := p.url
	p.mu.Unlock()

	log.Debug().Msgf("[streams] retry=%d to url=%s", retry, url)

	// Redial with p.mu released. GetProducer can block for a long time (exec
	// spawn plus stream probe, RTSP dial), and holding the mutex across it
	// freezes the whole producer: consumer Dial and GetTrack, and stop().
	// An exec pipe probe against a source that never writes blocks here
	// indefinitely, and the producer never recovers.
	conn, err := dialBounded(url)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.workerID != workerID {
		log.Trace().Msgf("[streams] stop reconnect url=%s", p.url)
		if conn != nil {
			_ = conn.Stop()
		}
		return
	}

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

	go p.worker(conn, workerID)
}

func (p *Producer) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == stateExternal {
		log.Trace().Msgf("[streams] skip stop external producer")
		return
	}

	// Invalidate any dial still in flight, in every state. A dial started for
	// a consumer that has since gone away must not install its connection
	// afterwards, because that leaves a running exec child with no consumers.
	// This came for free while Dial held p.mu across the whole dial.
	p.epoch++

	if p.state == stateNone {
		log.Trace().Msgf("[streams] skip stop none producer")
		return
	}

	if p.state == stateStart {
		p.workerID++
	}

	log.Debug().Msgf("[streams] stop producer url=%s", p.url)

	if p.conn != nil {
		_ = p.conn.Stop()
		p.conn = nil
	}

	p.state = stateNone
	p.receivers = nil
	p.senders = nil
}
