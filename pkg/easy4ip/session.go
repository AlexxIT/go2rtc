package easy4ip

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"time"
)

// deadAfter: a device silent this long means the session is dead (a variable for tests).
var deadAfter = 30 * time.Second

// bindTimeout: how long a new stream waits for the device to accept the port (a variable for tests).
var bindTimeout = 10 * time.Second

const (
	backoffStart = 2 * time.Second
	backoffMax   = 30 * time.Second
	heartbeat    = 5 * time.Second
	chunk        = 4096 // one Write is split into payloads of at most this size
)

// ErrClosed is returned by Dial after Close.
var ErrClosed = errors.New("easy4ip: client closed")

// Config of a device. The password is only used to sign channel requests.
type Config struct {
	Serial string
	Relay  bool // experimental: carry the session through the Dahua relay
	Cred   *Cred

	// OnState, if set, is called when the session comes up or is lost.
	OnState func(up bool)
}

// Client keeps one PTCP session to a device alive: handshake, run until the device goes silent,
// handshake again. A device accepts only one such session per client, so use one Client per device.
// Streams are TCP-like connections to a device port inside that session.
type Client struct {
	cfg Config

	mu      sync.Mutex
	cur     *live
	changed chan struct{} // closed and replaced whenever cur changes
	stop    chan struct{}
	stopped bool
}

// NewClient starts the supervisor; Close ends it.
func NewClient(cfg Config) *Client {
	c := &Client{cfg: cfg, changed: make(chan struct{}), stop: make(chan struct{})}
	go c.run()
	return c
}

// WaitReady blocks until a session is up; false on timeout or after Close.
func (c *Client) WaitReady(timeout time.Duration) bool {
	t := time.After(timeout)
	for {
		c.mu.Lock()
		up, ch, stopped := c.cur != nil, c.changed, c.stopped
		c.mu.Unlock()
		if up || stopped {
			return up
		}
		select {
		case <-ch:
		case <-t:
			return false
		case <-c.stop:
			return false
		}
	}
}

// Stats describes the current session: bytes moved so far modulo 2^32, and how many times each
// offset has wrapped. Zero values when no session is up.
type Stats struct {
	Recv, Sent           uint32
	RecvWraps, SentWraps int
}

func (c *Client) Stats() Stats {
	c.mu.Lock()
	l := c.cur
	c.mu.Unlock()
	if l == nil {
		return Stats{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return Stats{l.st.recv, l.st.sent, l.recvWraps, l.sentWraps}
}

// Ready reports whether a session is up.
func (c *Client) Ready() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cur != nil
}

func (c *Client) set(l *live) {
	c.mu.Lock()
	c.cur = l
	close(c.changed)
	c.changed = make(chan struct{})
	c.mu.Unlock()
}

func (c *Client) run() {
	backoff := backoffStart
	for {
		select {
		case <-c.stop:
			return
		default:
		}

		sock, st, err := openSession(c.cfg.Serial, c.cfg.Relay, c.cfg.Cred)
		if err != nil {
			warnf("handshake failed (%v), retrying in %v", err, backoff)
			select {
			case <-time.After(backoff):
			case <-c.stop:
				return
			}
			backoff = min(backoff*2, backoffMax)
			continue
		}
		backoff = backoffStart
		st.clearUnacked()

		l := newLive(sock, st)
		c.mu.Lock()
		if c.stopped {
			c.mu.Unlock()
			l.shutdown()
			return
		}
		c.mu.Unlock()
		c.set(l)
		logf("easy4ip: PTCP session established")
		if c.cfg.OnState != nil {
			c.cfg.OnState(true)
		}

		l.serve()

		// session dead: detach new streams, close existing ones so their players reconnect
		c.set(nil)
		l.shutdown()
		logf("easy4ip: PTCP session lost, reconnecting")
		if c.cfg.OnState != nil {
			c.cfg.OnState(false)
		}
	}
}

// Close tells the device the session is over (it otherwise keeps a dead session and falls off the
// P2P cloud for a while on quick restarts) and stops the supervisor.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return nil
	}
	c.stopped = true
	l := c.cur
	close(c.stop)
	c.mu.Unlock()

	if l != nil {
		l.emit(event{kind: evClose})
		time.Sleep(300 * time.Millisecond)
		l.shutdown()
	}
	return nil
}

// Dial opens a stream to a TCP port of the device (554 RTSP, 80 HTTP...). It waits for a session
// while one is being set up.
func (c *Client) Dial(port int) (*Stream, error) {
	for deadline := time.After(2 * HandshakeTimeout); ; {
		c.mu.Lock()
		l, ch := c.cur, c.changed
		stopped := c.stopped
		c.mu.Unlock()

		if stopped {
			return nil, ErrClosed
		}
		if l != nil {
			return l.open(port)
		}
		select {
		case <-ch:
		case <-deadline:
			return nil, errors.New("easy4ip: no P2P session available")
		case <-c.stop:
			return nil, ErrClosed
		}
	}
}

type eventKind byte

const (
	evHeartbeat eventKind = iota
	evConnect
	evDisconnect
	evData
	evClose // end the whole session, what DMSS sends to its previous session before reconnecting
)

type event struct {
	kind  eventKind
	realm uint32
	port  uint32
	data  []byte
}

// live is one established PTCP session.
type live struct {
	sock *peerConn

	mu sync.Mutex // guards st and the counters below
	st *ptcp

	lastRecv, lastSent uint32
	recvWraps          int
	sentWraps          int

	events chan event
	done   chan struct{}
	once   sync.Once

	streamsMu sync.Mutex
	streams   map[uint32]*Stream
}

func newLive(sock *peerConn, st *ptcp) *live {
	return &live{
		sock:    sock,
		st:      st,
		events:  make(chan event, 128),
		done:    make(chan struct{}),
		streams: map[uint32]*Stream{},
	}
}

// emit queues an event for the writer; false if the session is gone.
func (l *live) emit(ev event) bool {
	select {
	case <-l.done: // check first: with room in the queue both cases below would be ready
		return false
	default:
	}
	select {
	case l.events <- ev:
		return true
	case <-l.done:
		return false
	}
}

func (l *live) write(p *packet) {
	if _, err := l.sock.Write(p.marshal()); err != nil {
		// ICMP errors are transient while NAT mappings settle; retransmission recovers
		logf("easy4ip: send: %v", err)
	}
}

// serve runs the session until the device goes silent.
func (l *live) serve() {
	go l.writer()
	go l.heartbeat()
	go l.retransmitter()
	l.reader()
}

func (l *live) shutdown() {
	l.once.Do(func() {
		close(l.done)
		_ = l.sock.Close()
	})
	l.streamsMu.Lock()
	for realm, s := range l.streams {
		delete(l.streams, realm)
		s.eof()
	}
	l.streamsMu.Unlock()
}

func (l *live) heartbeat() {
	t := time.NewTicker(heartbeat)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if !l.emit(event{kind: evHeartbeat}) {
				return
			}
		case <-l.done:
			return
		}
	}
}

// writer turns events into packets.
func (l *live) writer() {
	for {
		var ev event
		select {
		case ev = <-l.events:
		case <-l.done:
			return
		}

		var b *body
		switch ev.kind {
		case evHeartbeat:
			b = &body{kind: kindHeartbeat}
		case evConnect:
			b = &body{kind: kindBind, realm: ev.realm, port: ev.port}
		case evDisconnect:
			l.streamsMu.Lock()
			delete(l.streams, ev.realm)
			l.streamsMu.Unlock()
			b = &body{kind: kindStatus, realm: ev.realm, status: "DISC"}
		case evData:
			b = &body{kind: kindPayload, realm: ev.realm, data: ev.data}
		case evClose:
			b = &body{kind: kindCommand, cmd: cmd(0x14, nil)}
		}

		l.mu.Lock()
		p := l.st.send(b)
		l.countWraps()
		l.mu.Unlock()
		l.write(p)
	}
}

// retransmitter resends packets the device has not acked within the RTO.
func (l *live) retransmitter() {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-l.done:
			return
		}
		l.mu.Lock()
		due := l.st.dueRetransmits()
		l.mu.Unlock()
		for _, p := range due {
			l.write(p)
		}
	}
}

// reader dispatches what the device sends; it returns when the device has been silent too long.
func (l *live) reader() {
	for {
		_ = l.sock.SetReadDeadline(time.Now().Add(deadAfter))
		p, err := l.sock.readPacket()
		if err != nil {
			warnf("device silent for %v or socket closed: %v", deadAfter, err)
			return
		}

		hasBody := p.body.size() > 0
		l.mu.Lock()
		bodies := l.st.accept(p)
		l.countWraps()
		// Ack every data-bearing packet, including duplicates and gaps: repeated acks of the same
		// offset are what tell the device to resend the hole.
		var ack *packet
		if hasBody {
			ack = l.st.send(&body{kind: kindEmpty})
		}
		l.mu.Unlock()
		if ack != nil {
			l.write(ack)
		}

		for _, b := range bodies {
			l.dispatch(b)
		}
	}
}

// countWraps notes when a byte offset passes 2^32 (about 4.5 h of a main stream). Caller holds l.mu.
func (l *live) countWraps() {
	if l.st.recv < l.lastRecv && seqDiff(l.st.recv, l.lastRecv) > 0 {
		l.recvWraps++
	}
	if l.st.sent < l.lastSent && seqDiff(l.st.sent, l.lastSent) > 0 {
		l.sentWraps++
	}
	l.lastRecv, l.lastSent = l.st.recv, l.st.sent
}

func (l *live) dispatch(b *body) {
	switch b.kind {
	case kindStatus:
		l.streamsMu.Lock()
		s := l.streams[b.realm]
		if s != nil && b.status == "DISC" {
			delete(l.streams, b.realm)
		}
		l.streamsMu.Unlock()
		if s == nil {
			return
		}
		if b.status == "CONN" || b.status == "DISC" {
			select {
			case s.bound <- b.status == "CONN":
			default:
			}
		}
		if b.status == "DISC" {
			logf("easy4ip: realm %08x closed by device", b.realm)
			s.eof()
		}
	case kindPayload:
		l.streamsMu.Lock()
		s := l.streams[b.realm]
		l.streamsMu.Unlock()
		if s == nil {
			return
		}
		// ponytail: blocking here backpressures every stream behind the slowest reader;
		// per-stream buffering if one stalled player must not freeze the others
		select {
		case s.rx <- b.data:
		case <-s.closed:
		case <-l.done:
		}
	}
}

// open binds a new realm to a device port.
func (l *live) open(port int) (*Stream, error) {
	var r [4]byte
	_, _ = rand.Read(r[:])
	s := &Stream{
		l:      l,
		realm:  binary.BigEndian.Uint32(r[:]),
		rx:     make(chan []byte, 128),
		bound:  make(chan bool, 1),
		closed: make(chan struct{}),
	}

	l.streamsMu.Lock()
	l.streams[s.realm] = s
	l.streamsMu.Unlock()

	fail := func(err error) (*Stream, error) {
		l.streamsMu.Lock()
		delete(l.streams, s.realm)
		l.streamsMu.Unlock()
		return nil, err
	}

	if !l.emit(event{kind: evConnect, realm: s.realm, port: uint32(port)}) {
		return fail(errors.New("easy4ip: session lost"))
	}
	select {
	case ok := <-s.bound:
		if !ok {
			return fail(errors.New("easy4ip: device refused port"))
		}
	case <-time.After(bindTimeout):
		return fail(errors.New("easy4ip: bind timeout"))
	case <-l.done:
		return fail(errors.New("easy4ip: session lost"))
	}
	return s, nil
}

// Stream is a TCP-like connection to a device port inside the session. It ends with io.EOF when the
// device closes it or the session is lost.
type Stream struct {
	l     *live
	realm uint32

	rx     chan []byte
	bound  chan bool
	rxOnce sync.Once

	closed    chan struct{}
	closeOnce sync.Once

	pending []byte
}

func (s *Stream) eof() { s.rxOnce.Do(func() { close(s.rx) }) }

func (s *Stream) Read(b []byte) (int, error) {
	if len(s.pending) == 0 {
		select {
		case data, ok := <-s.rx:
			if !ok {
				return 0, io.EOF
			}
			s.pending = data
		case <-s.closed:
			return 0, io.ErrClosedPipe
		}
	}
	n := copy(b, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

func (s *Stream) Write(b []byte) (int, error) {
	for sent := 0; sent < len(b); {
		select {
		case <-s.closed:
			return sent, io.ErrClosedPipe
		default:
		}
		n := min(len(b)-sent, chunk)
		if !s.l.emit(event{kind: evData, realm: s.realm, data: append([]byte(nil), b[sent:sent+n]...)}) {
			return sent, io.ErrClosedPipe
		}
		sent += n
	}
	return len(b), nil
}

func (s *Stream) Close() error {
	s.closeOnce.Do(func() {
		close(s.closed)
		s.l.emit(event{kind: evDisconnect, realm: s.realm})
	})
	return nil
}
