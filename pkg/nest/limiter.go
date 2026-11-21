package nest

import (
	"fmt"
	"sync"
	"time"
)

// Google enforces these limits on the Device Access API
// (https://developers.google.com/nest/device-access/project/limits):
//
//	devices.executeCommand   10 QPM per project, per user
//	each trait command        5 QPM per project, per user, per device
//	CAMERA / DOORBELL        30 QPM or 100 QPH per device
//	devices.list              5 QPM per project, per user
//
// Exceeding them returns 429 RESOURCE_EXHAUSTED, and every call that earned
// the 429 still counts. go2rtc can easily blow through the project-wide
// executeCommand budget: a restart with a dozen preloaded cameras, or a
// dashboard polling a few switched-off cameras, is enough. The limiter below
// reserves a slot in every applicable window before a request is sent and
// either sleeps briefly until the slot is free or, when the wait would be
// longer than the caller can afford, fails at once so a fallback source can
// take over instead of every consumer stalling.
const (
	executeCommandPerMinute = 10
	commandPerDevicePerMin  = 5
	devicePerMinute         = 30
	devicePerHour           = 100
	listPerMinute           = 5

	// how long a dial may wait for a slot before giving up; consumers such as
	// Home Assistant's snapshot fetch time out at 10 s
	dialMaxWait = 5 * time.Second
	// keep-alives run in the background and may wait for a slot
	extendMaxWait = 50 * time.Second
	// how long to hold off after Google actually answered 429
	penaltyAfter429 = time.Minute

	// slots per minute of the project budget that keep-alives and other
	// background calls leave free, so a dial - a consumer waiting on a
	// camera - is never starved by nine sessions extending at once
	dialReserve = 3
)

// ThrottledError is returned when a request would have had to wait longer
// than the caller allowed for a free slot.
type ThrottledError struct {
	Wait time.Duration
}

func (e *ThrottledError) Error() string {
	return fmt.Sprintf("nest: rate limited locally, next slot in %s", e.Wait.Round(time.Second))
}

// window is a sliding-window counter: at most limit calls in any span of per.
type window struct {
	limit int
	per   time.Duration
	times []time.Time // reserved call times, ascending
}

func (w *window) prune(now time.Time) {
	i := 0
	for i < len(w.times) && now.Sub(w.times[i]) >= w.per {
		i++
	}
	w.times = w.times[i:]
}

// earliest returns the soonest time at or after now at which one more call
// fits in the window.
func (w *window) earliest(now time.Time) time.Time {
	return w.earliestWithin(now, w.limit)
}

// earliestWithin is earliest for a caller allowed only `limit` of the
// window's slots.
func (w *window) earliestWithin(now time.Time, limit int) time.Time {
	w.prune(now)
	if len(w.times) < limit {
		return now
	}
	return w.times[len(w.times)-limit].Add(w.per)
}

func (w *window) add(t time.Time) {
	// keep ascending order; reservations are handed out in time order but a
	// penalty can push a later reservation past an earlier one
	i := len(w.times)
	for i > 0 && w.times[i-1].After(t) {
		i--
	}
	w.times = append(w.times, time.Time{})
	copy(w.times[i+1:], w.times[i:])
	w.times[i] = t
}

type limiter struct {
	mu      sync.Mutex
	windows map[string]*window
	penalty map[string]time.Time // projectID -> hold off until

	now   func() time.Time
	sleep func(time.Duration)
}

func newLimiter() *limiter {
	return &limiter{
		windows: map[string]*window{},
		penalty: map[string]time.Time{},
		now:     time.Now,
		sleep:   time.Sleep,
	}
}

var limits = newLimiter()

func (l *limiter) window(key string, limit int, per time.Duration) *window {
	w, ok := l.windows[key]
	if !ok {
		w = &window{limit: limit, per: per}
		l.windows[key] = w
	}
	return w
}

// reserve finds the earliest time at which a call fits every window, books
// that slot in all of them, and returns how long the caller must wait. If the
// wait exceeds maxWait nothing is booked and a ThrottledError is returned.
func (l *limiter) reserve(windows []*window, projectID string, maxWait time.Duration) (time.Duration, error) {
	return l.reserveWithin(windows, nil, projectID, maxWait)
}

// reserveWithin is reserve with a per-window cap on how many of its slots
// this caller may use; a nil or missing entry means all of them.
func (l *limiter) reserveWithin(windows []*window, caps map[*window]int, projectID string, maxWait time.Duration) (time.Duration, error) {
	now := l.now()
	t := now
	if until, ok := l.penalty[projectID]; ok {
		if until.After(t) {
			t = until
		} else {
			delete(l.penalty, projectID)
		}
	}
	for _, w := range windows {
		limit := w.limit
		if c, ok := caps[w]; ok {
			limit = c
		}
		if e := w.earliestWithin(now, limit); e.After(t) {
			t = e
		}
	}
	wait := t.Sub(now)
	if wait > maxWait {
		return 0, &ThrottledError{Wait: wait}
	}
	for _, w := range windows {
		w.add(t)
	}
	return wait, nil
}

// Command books a slot for one devices.executeCommand call and sleeps until
// it is free, or returns a ThrottledError if that would take longer than
// maxWait.
func (l *limiter) Command(projectID, deviceID, command string, maxWait time.Duration) error {
	l.mu.Lock()
	windows := []*window{
		l.window("cmd:"+projectID, executeCommandPerMinute, time.Minute),
		l.window("cmd:"+projectID+":"+deviceID+":"+command, commandPerDevicePerMin, time.Minute),
		l.window("dev:"+projectID+":"+deviceID+":m", devicePerMinute, time.Minute),
		l.window("dev:"+projectID+":"+deviceID+":h", devicePerHour, time.Hour),
	}
	wait, err := l.reserve(windows, projectID, maxWait)
	l.mu.Unlock()
	if err != nil {
		return err
	}
	if wait > 0 {
		l.sleep(wait)
	}
	return nil
}

// CommandBackground is Command for keep-alives and other calls no consumer
// is waiting on: it leaves dialReserve slots of the project's per-minute
// budget free for dials.
func (l *limiter) CommandBackground(projectID, deviceID, command string, maxWait time.Duration) error {
	l.mu.Lock()
	project := l.window("cmd:"+projectID, executeCommandPerMinute, time.Minute)
	windows := []*window{
		project,
		l.window("cmd:"+projectID+":"+deviceID+":"+command, commandPerDevicePerMin, time.Minute),
		l.window("dev:"+projectID+":"+deviceID+":m", devicePerMinute, time.Minute),
		l.window("dev:"+projectID+":"+deviceID+":h", devicePerHour, time.Hour),
	}
	caps := map[*window]int{project: executeCommandPerMinute - dialReserve}
	wait, err := l.reserveWithin(windows, caps, projectID, maxWait)
	l.mu.Unlock()
	if err != nil {
		return err
	}
	if wait > 0 {
		l.sleep(wait)
	}
	return nil
}

// List books a slot for one devices.list call.
func (l *limiter) List(projectID string, maxWait time.Duration) error {
	l.mu.Lock()
	windows := []*window{l.window("list:"+projectID, listPerMinute, time.Minute)}
	wait, err := l.reserve(windows, projectID, maxWait)
	l.mu.Unlock()
	if err != nil {
		return err
	}
	if wait > 0 {
		l.sleep(wait)
	}
	return nil
}

// Penalize holds off every call for the project after Google answered 429:
// the quota window is opaque to us, so wait it out rather than probe it.
func (l *limiter) Penalize(projectID string, d time.Duration) {
	l.mu.Lock()
	until := l.now().Add(d)
	if cur, ok := l.penalty[projectID]; !ok || until.After(cur) {
		l.penalty[projectID] = until
	}
	l.mu.Unlock()
}
