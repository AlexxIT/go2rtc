package miss

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"
)

type sessionKey struct {
	did          string
	host         string
	user         string
	model        string
	vendor       string
	endpoint     string
	devicePublic string
}

type sessionSlot struct {
	mu   sync.Mutex
	refs int
	mux  *mux
}

var (
	sessions           = map[sessionKey]*sessionSlot{}
	sessionsMu         sync.Mutex
	errMuxIncompatible = errors.New("xiaomi: incompatible mux quality")
)

func parseChannel(value string) (uint8, error) {
	switch value {
	case "", "0":
		return 0, nil
	case "1", "2":
		return 1, nil
	default:
		return 0, fmt.Errorf("xiaomi: unsupported channel %q", value)
	}
}

func makeSessionKey(u *url.URL, query url.Values, did string) (sessionKey, error) {
	vendor := query.Get("vendor")
	if vendor == "" || query.Get("client_public") == "" || query.Get("sign") == "" {
		return sessionKey{}, errMuxIncompatible
	}
	var endpoint string
	switch vendor {
	case "cs2":
		endpoint = query.Get("transport")
	case "tutk":
		endpoint = query.Get("uid")
	default:
		return sessionKey{}, errMuxIncompatible
	}
	var user string
	if u.User != nil {
		user = u.User.String()
	}
	return sessionKey{
		did:          did,
		host:         u.Host,
		user:         user,
		model:        query.Get("model"),
		vendor:       vendor,
		endpoint:     endpoint,
		devicePublic: query.Get("device_public"),
	}, nil
}

func releaseSessionSlot(key sessionKey, s *sessionSlot) {
	sessionsMu.Lock()
	s.refs--
	if s.refs == 0 && sessions[key] == s {
		delete(sessions, key)
	}
	sessionsMu.Unlock()
}

func dialSharedClient(u *url.URL, query url.Values, did string, sharedKey []byte) (*Client, error) {
	ch, err := parseChannel(query.Get("channel"))
	if err != nil {
		return nil, err
	}
	key, err := makeSessionKey(u, query, did)
	if err != nil {
		return nil, err
	}
	quality := query.Get("subtype")
	wantAudio := query.Get("audio") != "0"

	sessionsMu.Lock()
	s := sessions[key]
	if s == nil {
		s = &sessionSlot{}
		sessions[key] = s
	}
	s.refs++
	sessionsMu.Unlock()
	defer releaseSessionSlot(key, s)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.mux != nil {
		client, err := s.mux.attach(ch, quality, wantAudio)
		if err == nil {
			return client, nil
		}
		if !errors.Is(err, io.EOF) {
			return nil, err
		}
		s.mux = nil
	}

	realClient, err := dialClient(u, query, sharedKey)
	if err != nil {
		return nil, err
	}

	m := newMux(realClient)
	client, err := m.attach(ch, quality, wantAudio)
	if err != nil {
		_ = m.close(nil, false)
		return nil, err
	}

	sessionsMu.Lock()
	s.refs++
	sessionsMu.Unlock()

	m.mu.Lock()
	closed := m.closed
	if !closed {
		m.onClose = func() {
			s.mu.Lock()
			if s.mux == m {
				s.mux = nil
			}
			s.mu.Unlock()
			releaseSessionSlot(key, s)
		}
		s.mux = m
	}
	m.mu.Unlock()
	if closed {
		releaseSessionSlot(key, s)
	}

	return client, nil
}

type mux struct {
	client  *Client
	startMu sync.Mutex

	mu           sync.Mutex
	closed       bool
	err          error
	done         chan struct{}
	onClose      func()
	clients      []*muxClient
	startedMask  uint8
	audioEnabled bool
	quality      [2]string
	lensTagKnown bool
	lensTag      uint32
	lensChannel  uint8
	lensTagReady chan struct{}
}

type muxClient struct {
	mux     *mux
	channel uint8
	quality string
	audio   bool

	packets    chan *Packet
	done       chan struct{}
	mu         sync.Mutex
	closed     bool
	deadline   time.Time
	deadlineCh chan struct{}
}

func newMux(client *Client) *mux {
	m := &mux{
		client:       client,
		done:         make(chan struct{}),
		lensTagReady: make(chan struct{}),
	}
	go m.readLoop()
	return m
}

func (m *mux) attach(ch uint8, quality string, audio bool) (*Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, io.EOF
	}

	q := m.client.parseQuality(quality)
	if m.startedMask&(1<<ch) != 0 && m.quality[ch] != q {
		return nil, errMuxIncompatible
	}
	for _, cur := range m.clients {
		if cur.channel == ch && cur.quality != q {
			return nil, errMuxIncompatible
		}
	}

	sub := &muxClient{
		mux:        m,
		channel:    ch,
		quality:    q,
		audio:      audio,
		packets:    make(chan *Packet, 64),
		done:       make(chan struct{}),
		deadlineCh: make(chan struct{}),
	}
	m.clients = append(m.clients, sub)

	return &Client{
		Conn:  m.client.Conn,
		model: m.client.model,
		sub:   sub,
	}, nil
}

func (m *mux) readLoop() {
	for {
		_ = m.client.SetDeadline(time.Now().Add(15 * time.Second))
		pkt, err := m.client.ReadPacket()
		if err != nil {
			_ = m.close(err, false)
			return
		}
		m.dispatch(pkt)
	}
}

func (m *mux) dispatch(pkt *Packet) {
	var isVideo bool
	switch pkt.CodecID {
	case codecH264, codecH265:
		isVideo = true
	case codecPCM, codecPCMU, codecPCMA, codecOPUS:
	default:
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return
	}

	var ch uint8
	if isVideo {
		var ok bool
		if ch, ok = m.resolveChannel(pkt); !ok {
			return
		}
	}

	for _, c := range m.clients {
		if (isVideo && c.channel == ch) || (!isVideo && c.audio) {
			select {
			case c.packets <- pkt:
			default:
			}
		}
	}
}

func (m *mux) resolveChannel(pkt *Packet) (uint8, bool) {
	if m.client != nil && m.client.usesLensTags() {
		// Measured CW500 flags: 0x000EA000 / 0x014EA000 (HD),
		// 0x0006A000 / 0x0146A000 (SD).
		tag := (pkt.Flags >> 16) & 0x0140
		if tag != 0 && tag != 0x0140 {
			return 0, false
		}
		if !m.lensTagKnown {
			switch m.startedMask {
			case 1:
				m.lensChannel = 0
			case 2:
				m.lensChannel = 1
			default:
				return 0, false
			}
			m.lensTag = tag
			m.lensTagKnown = true
			close(m.lensTagReady)
		}
		if tag == m.lensTag {
			return m.lensChannel, true
		}
		return m.lensChannel ^ 1, true
	}
	if pkt.ChannelOK {
		return pkt.Channel, true
	}
	switch m.startedMask {
	case 1:
		return 0, true
	case 2:
		return 1, true
	default:
		return 0, false
	}
}

func (m *mux) closeLocked(err error, stop bool) (*Client, bool, func()) {
	if m.closed {
		return nil, false, nil
	}
	m.closed = true
	m.err = err
	close(m.done)
	doStop := stop && m.client != nil && m.startedMask != 0
	m.clients = nil
	onClose := m.onClose
	m.onClose = nil
	return m.client, doStop, onClose
}

func (m *mux) close(err error, stop bool) error {
	m.mu.Lock()
	client, doStop, onClose := m.closeLocked(err, stop)
	m.mu.Unlock()
	return closeUnderlying(client, doStop, onClose)
}

func closeUnderlying(client *Client, stop bool, onClose func()) error {
	if onClose != nil {
		defer onClose()
	}
	if client == nil {
		return nil
	}
	if stop {
		_ = client.StopMedia()
	}
	return client.Close()
}

func (s *muxClient) startMedia() error {
	m := s.mux
	m.startMu.Lock()
	defer m.startMu.Unlock()

	ch := s.channel
	var prevStarted uint8
	var prevQuality [2]string
	var prevAudio, otherStarted, audioOn bool
	var q [2]string

	for {
		select {
		case <-s.done:
			return io.EOF
		default:
		}

		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return io.EOF
		}
		prevStarted = m.startedMask
		prevQuality = m.quality
		prevAudio = m.audioEnabled

		alreadyStarted := prevStarted&(1<<ch) != 0
		otherStarted = prevStarted&(1<<(ch^1)) != 0
		if !alreadyStarted && otherStarted && m.client.usesLensTags() && !m.lensTagKnown {
			m.mu.Unlock()
			timer := time.NewTimer(15 * time.Second)
			select {
			case <-m.lensTagReady:
				timer.Stop()
				continue
			case <-s.done:
				timer.Stop()
				return io.EOF
			case <-m.done:
				timer.Stop()
				if m.err != nil {
					return m.err
				}
				return io.EOF
			case <-timer.C:
				return os.ErrDeadlineExceeded
			}
		}

		if alreadyStarted && (!s.audio || prevAudio) {
			m.mu.Unlock()
			return nil
		}
		if !alreadyStarted {
			m.startedMask |= 1 << ch
			m.quality[ch] = s.quality
		}
		if s.audio {
			m.audioEnabled = true
		}
		q = m.quality
		audioOn = m.audioEnabled
		m.mu.Unlock()
		break
	}

	a := "0"
	if audioOn {
		a = "1"
	}
	var err error
	if otherStarted {
		err = m.client.startMediaDual(q[0], q[1], a)
	} else {
		err = m.client.StartMedia(strconv.Itoa(int(ch)), q[ch], a)
	}
	if err != nil {
		m.mu.Lock()
		m.startedMask = prevStarted
		m.quality = prevQuality
		m.audioEnabled = prevAudio
		m.mu.Unlock()
	}
	return err
}

func (s *muxClient) isClosed() bool {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	return closed
}

func (s *muxClient) writeCommand(data []byte) error {
	if s.isClosed() {
		return io.EOF
	}
	return s.mux.client.WriteCommand(data)
}

func (s *muxClient) writeAudio(codecID uint32, payload []byte) error {
	if s.isClosed() {
		return io.EOF
	}
	return s.mux.client.WriteAudio(codecID, payload)
}

func (s *muxClient) setDeadline(t time.Time) error {
	s.mu.Lock()
	s.deadline = t
	close(s.deadlineCh)
	s.deadlineCh = make(chan struct{})
	s.mu.Unlock()
	return nil
}

func (s *muxClient) readPacket() (*Packet, error) {
	for {
		s.mu.Lock()
		closed, deadline, ch := s.closed, s.deadline, s.deadlineCh
		s.mu.Unlock()
		if closed {
			return nil, io.EOF
		}

		pkt, retry, err := s.waitPacket(deadline, ch)
		if !retry {
			return pkt, err
		}
	}
}

func (s *muxClient) waitPacket(deadline time.Time, ch <-chan struct{}) (*Packet, bool, error) {
	var timeout <-chan time.Time
	if !deadline.IsZero() {
		d := time.Until(deadline)
		if d <= 0 {
			return nil, false, os.ErrDeadlineExceeded
		}
		timer := time.NewTimer(d)
		defer timer.Stop()
		timeout = timer.C
	}

	select {
	case pkt := <-s.packets:
		return pkt, false, nil
	case <-ch:
		return nil, true, nil
	case <-s.done:
		return nil, false, io.EOF
	case <-s.mux.done:
		if s.mux.err != nil {
			return nil, false, s.mux.err
		}
		return nil, false, io.EOF
	case <-timeout:
		return nil, false, os.ErrDeadlineExceeded
	}
}

func (s *muxClient) close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.done)
	s.mu.Unlock()

	m := s.mux
	m.mu.Lock()
	for i, cur := range m.clients {
		if cur == s {
			m.clients = append(m.clients[:i], m.clients[i+1:]...)
			break
		}
	}
	var client *Client
	var doStop bool
	var onClose func()
	if len(m.clients) == 0 {
		client, doStop, onClose = m.closeLocked(nil, true)
	}
	m.mu.Unlock()

	return closeUnderlying(client, doStop, onClose)
}
