package unifiprotect

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/unifiprotect"
)

const (
	sourceTimeout   = 25 * time.Second
	probeTimeout    = 5 * time.Second
	settleTime      = 800 * time.Millisecond
	mediaProbeLimit = 8 * 1024 * 1024
)

var cameraChannels = [...]string{"video1", "video2", "video3"}

type streamKey struct {
	mac     string
	channel string
}

type streamRequest struct {
	key       streamKey
	token     string
	cameraIP  string
	audioMode unifiprotect.AudioMode

	// Lifecycle: pending -> probing -> committed -> released. Pending and
	// probing requests accept media candidates. Committing retires candidates,
	// while the request remains active until the producer is released.
	candidates        chan candidate
	candidatesDone    chan struct{}
	mu                sync.Mutex
	candidatesRetired bool
	releaseOnce       sync.Once
}

type candidate struct {
	conn net.Conn
	rd   io.ReadCloser
}

type Manager struct {
	mu sync.Mutex

	sessions map[string]*session
	pending  map[string]*streamRequest
	active   map[streamKey]*streamRequest
	changed  chan struct{}

	mediaHost         string
	mediaPort         int
	controllerUUID    string
	controllerVersion string
	mediaProbeLimit   int64
}

func newManager(mediaHost string, mediaPort int, controllerVersion, controllerUUID string) *Manager {
	return &Manager{
		sessions:          make(map[string]*session),
		pending:           make(map[string]*streamRequest),
		active:            make(map[streamKey]*streamRequest),
		changed:           make(chan struct{}),
		mediaHost:         mediaHost,
		mediaPort:         mediaPort,
		controllerUUID:    controllerUUID,
		controllerVersion: controllerVersion,
		mediaProbeLimit:   mediaProbeLimit,
	}
}

func (m *Manager) addSession(s *session) {
	m.mu.Lock()
	old := m.sessions[s.mac]
	if old == s && s.ready {
		m.mu.Unlock()
		return
	}
	// Install the replacement before closing the old session. removeSession
	// checks identity, so a late close can't remove the new session. Keep it
	// unready until channels left behind by the previous connection are stopped.
	s.ready = false
	m.sessions[s.mac] = s
	active := make(map[string]bool)
	for key := range m.active {
		if key.mac == s.mac {
			active[key.channel] = true
		}
	}
	m.mu.Unlock()

	if old != nil && old != s {
		old.close()
	}

	m.mu.Lock()
	current := m.sessions[s.mac] == s
	m.mu.Unlock()
	if !current {
		return
	}

	var inactive []string
	for _, channel := range cameraChannels {
		if !active[channel] {
			inactive = append(inactive, channel)
		}
	}
	if err := s.stopStreams(inactive); err != nil {
		log.Debug().Err(err).Str("camera", s.mac).Msg("[unifi-protect] quiesce streams")
	}

	m.mu.Lock()
	if m.sessions[s.mac] != s {
		m.mu.Unlock()
		return
	}
	s.ready = true
	m.signalLocked()
	m.mu.Unlock()

	log.Info().Str("camera", s.mac).Str("remote", s.cameraIP).Msg("[unifi-protect] camera online")
}

func (m *Manager) removeSession(s *session) {
	m.mu.Lock()
	if m.sessions[s.mac] == s {
		delete(m.sessions, s.mac)
		m.signalLocked()
	}
	m.mu.Unlock()
}

func (m *Manager) signalLocked() {
	close(m.changed)
	m.changed = make(chan struct{})
}

func (m *Manager) waitSession(mac string, deadline time.Time) (*session, error) {
	for {
		m.mu.Lock()
		if s := m.sessions[mac]; s != nil && s.ready {
			m.mu.Unlock()
			return s, nil
		}
		changed := m.changed
		m.mu.Unlock()

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("%w: %s", errSessionUnavailable, mac)
		}
		timer := time.NewTimer(remaining)
		select {
		case <-changed:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			return nil, fmt.Errorf("%w: %s", errSessionUnavailable, mac)
		}
	}
}

func (m *Manager) open(source string) (core.Producer, error) {
	parsed, err := parseSource(source)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(sourceTimeout)
	s, err := m.waitSession(parsed.mac, deadline)
	if err != nil {
		return nil, err
	}

	host := m.mediaHost
	if host == "" {
		host = s.controllerHost
	}
	if host == "" {
		return nil, errors.New("unifi-protect: can't determine media destination host")
	}

	token, err := randomToken()
	if err != nil {
		return nil, err
	}

	audioMode := unifiprotect.AudioNone
	if parsed.audio {
		if s.opusRate != 0 {
			audioMode = unifiprotect.AudioOpus
		} else {
			audioMode = unifiprotect.AudioAAC
		}
	}
	req := &streamRequest{
		key:            streamKey{mac: parsed.mac, channel: parsed.channel},
		token:          token,
		cameraIP:       s.cameraIP,
		audioMode:      audioMode,
		candidates:     make(chan candidate, 1),
		candidatesDone: make(chan struct{}),
	}

	m.mu.Lock()
	if current := m.active[req.key]; current != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("unifi-protect: stream already active for %s/%s", parsed.mac, parsed.channel)
	}
	m.active[req.key] = req
	m.pending[token] = req
	m.mu.Unlock()

	destination := "tcp://" + net.JoinHostPort(host, strconv.Itoa(m.mediaPort))
	log.Debug().Str("camera", parsed.mac).Str("channel", parsed.channel).Str("destination", destination).Msg("[unifi-protect] start stream")
	if err = s.startStream(parsed.channel, destination, token, parsed.audio); err != nil {
		m.release(req, false)
		return nil, err
	}

	prod, err := m.waitProducer(req, deadline)
	if err != nil {
		m.release(req, true)
		return nil, err
	}
	prod.SetOnStop(func() { m.release(req, true) })
	prod.Source = source
	prod.RemoteAddr = s.cameraIP
	return prod, nil
}

func (m *Manager) waitProducer(req *streamRequest, deadline time.Time) (*unifiprotect.Producer, error) {
	var lastErr error

	for time.Now().Before(deadline) {
		selected, err := req.settledCandidate(deadline)
		if err != nil {
			if lastErr != nil {
				return nil, fmt.Errorf("unifi-protect: media probe: %w", lastErr)
			}
			return nil, err
		}

		probeDeadline := time.Now().Add(probeTimeout)
		if deadline.Before(probeDeadline) {
			probeDeadline = deadline
		}
		_ = selected.conn.SetReadDeadline(probeDeadline)
		prod, err := unifiprotect.Open(selected.rd, req.audioMode)
		if err != nil {
			lastErr = err
			continue
		}
		_ = selected.conn.SetReadDeadline(time.Time{})
		// Open owns the selected connection now. Retiring candidate intake closes
		// late duplicate pushes without touching the producer transport.
		req.retireCandidates()

		m.mu.Lock()
		if m.pending[req.token] == req {
			delete(m.pending, req.token)
		}
		m.mu.Unlock()
		return prod, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("unifi-protect: media probe: %w", lastErr)
	}
	return nil, errors.New("unifi-protect: timed out waiting for camera media")
}

func (r *streamRequest) settledCandidate(deadline time.Time) (candidate, error) {
	var selected candidate

	remaining := time.Until(deadline)
	if remaining <= 0 {
		return selected, errors.New("unifi-protect: timed out waiting for camera media")
	}
	deadlineTimer := time.NewTimer(remaining)
	defer deadlineTimer.Stop()
	select {
	case selected = <-r.candidates:
	case <-r.candidatesDone:
		return selected, net.ErrClosed
	case <-deadlineTimer.C:
		return selected, errors.New("unifi-protect: timed out waiting for camera media")
	}

	// A camera may open several connections for one request. Let the burst
	// settle and keep the newest attempt, closing the ones it supersedes.
	settle := time.NewTimer(settleTime)
	defer settle.Stop()
	for {
		select {
		case next := <-r.candidates:
			_ = selected.rd.Close()
			selected = next
			if !settle.Stop() {
				<-settle.C
			}
			settle.Reset(settleTime)
		case <-settle.C:
			return selected, nil
		case <-r.candidatesDone:
			_ = selected.rd.Close()
			return candidate{}, net.ErrClosed
		case <-deadlineTimer.C:
			_ = selected.rd.Close()
			return candidate{}, errors.New("unifi-protect: timed out waiting for camera media")
		}
	}
}

func (r *streamRequest) offerCandidate(c candidate) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.candidatesRetired {
		return false
	}
	select {
	case old := <-r.candidates:
		_ = old.rd.Close()
	default:
	}
	r.candidates <- c
	return true
}

func (r *streamRequest) retireCandidates() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.candidatesRetired {
		return
	}
	r.candidatesRetired = true
	close(r.candidatesDone)
	select {
	case c := <-r.candidates:
		_ = c.rd.Close()
	default:
	}
}

func (m *Manager) release(req *streamRequest, stop bool) {
	req.releaseOnce.Do(func() {
		req.retireCandidates()

		m.mu.Lock()
		if m.active[req.key] != req {
			m.mu.Unlock()
			return
		}
		if m.pending[req.token] == req {
			delete(m.pending, req.token)
		}
		s := m.sessions[req.key.mac]
		m.mu.Unlock()

		// Keep the request active until the stop command completes. Otherwise a
		// replacement could start the same channel while its old push still runs.
		if stop && s != nil {
			if err := s.stopStreams([]string{req.key.channel}); err != nil {
				log.Debug().Err(err).Str("camera", req.key.mac).Str("channel", req.key.channel).Msg("[unifi-protect] stop stream")
			}
		}

		m.mu.Lock()
		if m.active[req.key] == req {
			delete(m.active, req.key)
		}
		m.mu.Unlock()
	})
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
