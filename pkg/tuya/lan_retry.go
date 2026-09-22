package tuya

import (
	"sync"
	"time"
)

// The camera hands out only a couple of concurrent media sessions at a time and keeps each one
// occupied for a while after the client goes away. When a new session is refused, the WebRTC
// handshake, the offer and the ICE negotiation all still complete - the camera just never sends
// media - so the failure looks like a healthy session that stays empty.
//
// go2rtc restarts a producer as soon as a consumer asks again, and a producer whose Start()
// fails is retried immediately (its retry counter restarts at zero), so an ungraceful retry loop
// keeps the camera saturated with half sessions and the `tuya-lan` source stays dead until the
// process is restarted. Spacing failed attempts out lets the camera's session table drain.

const (
	lanRetryMin = 2 * time.Second
	lanRetryMax = 2 * time.Minute
)

type lanRetryState struct {
	mu       sync.Mutex
	failures int
	until    time.Time
}

// Retry state is per physical camera. A package-global window makes one offline
// camera suppress every other tuya-lan source in the process, even though their
// session tables are independent.
type lanRetryRegistry struct {
	mu       sync.Mutex
	byDevice map[string]*lanRetryState
}

func (r *lanRetryRegistry) For(deviceID string) *lanRetryState {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.byDevice == nil {
		r.byDevice = make(map[string]*lanRetryState)
	}
	state := r.byDevice[deviceID]
	if state == nil {
		state = new(lanRetryState)
		r.byDevice[deviceID] = state
	}
	return state
}

var lanRetries lanRetryRegistry

// Allow reports whether a new session may be opened now. While the window is active the dial is
// rejected without touching the camera: every attempt, successful or not, occupies one of the
// camera's few session slots for the whole drain interval, so hammering it is what keeps the
// source dead.
func (s *lanRetryState) Allow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return !time.Now().Before(s.until)
}

// Remaining reports how much of the current backoff window is left, for the error message.
func (s *lanRetryState) Remaining() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	if d := time.Until(s.until); d > 0 {
		return d.Round(time.Second)
	}
	return 0
}

// Failed records a failed session attempt and doubles the next backoff window.
func (s *lanRetryState) Failed() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.failures++
	s.until = time.Now().Add(lanRetryDelay(s.failures))
}

// OK clears the backoff after a session that really carries media.
func (s *lanRetryState) OK() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.failures = 0
	s.until = time.Time{}
}

func lanRetryDelay(failures int) time.Duration {
	delay := lanRetryMin
	for i := 1; i < failures; i++ {
		delay *= 2
		if delay >= lanRetryMax {
			return lanRetryMax
		}
	}
	return delay
}
