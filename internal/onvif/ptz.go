package onvif

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/onvif"
)

// ONVIF pan/tilt for sources that can move but speak no ONVIF themselves (the
// Xiaomi cameras, via their MISS motor command). Continuous moves are turned into
// a train of discrete motor steps until Stop -- or until maxMove, in case the
// NVR never sends Stop (a dropped request must not leave the camera spinning).

// motorer is any live source that can move its camera one step
// (1 - left, 2 - right, 3 - up, 4 - down). Duck-typed, so this module does not
// import the vendor modules.
type motorer interface {
	Motor(operation int) error
}

const (
	opLeft  = 1
	opRight = 2
	opUp    = 3
	opDown  = 4
)

// maxMove caps one ContinuousMove without a Stop.
const maxMove = 10 * time.Second

func initPTZ() {
	onvif.PTZ = hasMotor
	onvif.PTZAny = func() bool {
		for _, name := range streams.GetAllNames() {
			if hasMotor(name) {
				return true
			}
		}
		return false
	}
}

// hasMotor reports whether the stream is configured with a source that can
// pan/tilt. It looks at the config, not at live connections, so the ONVIF
// profile stays stable while the camera reconnects.
func hasMotor(name string) bool {
	stream := streams.Get(name)
	if stream == nil {
		return false
	}
	for _, src := range stream.Sources() {
		if strings.HasPrefix(src, "xiaomi:") {
			return true
		}
	}
	return false
}

func motor(name string, operation int) error {
	if stream := streams.Get(name); stream != nil {
		for _, conn := range stream.Conns() {
			if m, ok := conn.(motorer); ok {
				return m.Motor(operation)
			}
		}
	}
	return errNoMotor
}

type ptzError string

func (e ptzError) Error() string { return string(e) }

const errNoMotor = ptzError("onvif: ptz: stream has no connected source with a motor")

// move is one running ContinuousMove. The pointer identifies the owner: a
// goroutine only removes its own entry, never a replacement's (see endMove).
type move struct {
	cancel context.CancelFunc
}

var (
	moves   = map[string]*move{}
	movesMu sync.Mutex
)

// stopMove cancels whatever move is running for the stream (Stop, or a new
// ContinuousMove replacing the old one).
func stopMove(name string) {
	movesMu.Lock()
	if m := moves[name]; m != nil {
		m.cancel()
		delete(moves, name)
	}
	movesMu.Unlock()
}

// endMove is the move goroutine's own cleanup. By the time it runs, a newer
// ContinuousMove may already own the map entry, so only remove it if it is
// still ours.
func endMove(name string, m *move) {
	movesMu.Lock()
	if moves[name] == m {
		delete(moves, name)
	}
	movesMu.Unlock()
	m.cancel()
}

func isMoving(name string) bool {
	movesMu.Lock()
	defer movesMu.Unlock()
	return moves[name] != nil
}

func startMove(name string, x, y float64) {
	stopMove(name)

	ops, interval := motorSteps(x, y)
	if len(ops) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), maxMove)
	m := &move{cancel: cancel}
	movesMu.Lock()
	moves[name] = m
	movesMu.Unlock()

	go func() {
		defer endMove(name, m)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			for _, op := range ops {
				if err := motor(name, op); err != nil {
					log.Warn().Err(err).Str("stream", name).Msg("[onvif] ptz")
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// motorSteps turns an ONVIF continuous velocity into the motor operations to
// send on each tick, and how often to repeat them while the move lasts.
//
// x: -1..1, negative = left, positive = right.
// y: -1..1, negative = down, positive = up.
// Frigate's arrows send exactly one axis at ±0.5; other clients may send both
// axes (a diagonal) or small values from a joystick's resting drift.
//
// Measured on the Mi 360 2K (2026-09-28): one step pans ~70 px of a 640 px frame
// (~11% of the view) and tilts ~40 px; 5 steps 400 ms apart all landed (345 px),
// so 400 ms is the fastest proven rate. Goal: feel like a native ONVIF camera
// (Tapo) -- moves while the arrow is held, stops on release, and at most one step
// is still in flight when Stop arrives.
func motorSteps(x, y float64) (ops []int, interval time.Duration) {
	const deadzone = 0.1 // joystick drift is not a request to move

	switch {
	case x <= -deadzone:
		ops = append(ops, opLeft)
	case x >= deadzone:
		ops = append(ops, opRight)
	}
	switch {
	case y >= deadzone:
		ops = append(ops, opUp)
	case y <= -deadzone:
		ops = append(ops, opDown)
	}
	if len(ops) == 0 {
		return nil, 0
	}

	// Speed from magnitude, like a native camera: Frigate's 0.5 gets the fastest
	// proven rate (400 ms/step), a slower joystick push gets longer gaps.
	v := math.Max(math.Abs(x), math.Abs(y))
	interval = time.Duration(float64(200*time.Millisecond) / v)
	interval = max(400*time.Millisecond, min(interval, 1200*time.Millisecond))
	return ops, interval
}

// The PanTilt element of a ContinuousMove, then its x and y attributes. They are
// matched separately because attribute order is not significant in XML and
// clients differ in the order (and quote style) they serialise.
var (
	rePanTilt = regexp.MustCompile(`<[^>]*\bPanTilt\b[^>]*>`)
	reX       = regexp.MustCompile(`\bx=["']([^"']*)["']`)
	reY       = regexp.MustCompile(`\by=["']([^"']*)["']`)
)

func parseVelocity(b []byte) (x, y float64) {
	tag := rePanTilt.Find(b)
	if tag == nil {
		return
	}
	if m := reX.FindSubmatch(tag); m != nil {
		x, _ = strconv.ParseFloat(string(m[1]), 64)
	}
	if m := reY.FindSubmatch(tag); m != nil {
		y, _ = strconv.ParseFloat(string(m[1]), 64)
	}
	return
}

// ptzOperation answers the PTZ SOAP operations; ok=false if it is not one.
func ptzOperation(operation string, body []byte) (b []byte, ok bool) {
	switch operation {
	case onvif.PTZGetConfigurationOptions:
		return onvif.GetPTZConfigurationOptionsResponse(), true
	case onvif.PTZGetConfigurations:
		return onvif.GetPTZConfigurationsResponse(streams.GetAllNames()), true
	case onvif.PTZGetConfiguration:
		return onvif.GetPTZConfigurationResponse(onvif.FindTagValue(body, "PTZConfigurationToken")), true
	case onvif.PTZGetNodes:
		return onvif.GetPTZNodesResponse(streams.GetAllNames()), true
	case onvif.PTZGetNode:
		return onvif.GetPTZNodeResponse(onvif.FindTagValue(body, "NodeToken")), true
	case onvif.PTZGetStatus:
		return onvif.GetPTZStatusResponse(isMoving(onvif.FindTagValue(body, "ProfileToken"))), true
	case onvif.PTZGetPresets:
		return onvif.PTZStaticResponse(operation), true
	case onvif.PTZContinuousMove:
		x, y := parseVelocity(body)
		startMove(onvif.FindTagValue(body, "ProfileToken"), x, y)
		return onvif.PTZStaticResponse(operation), true
	case onvif.PTZStop:
		stopMove(onvif.FindTagValue(body, "ProfileToken"))
		return onvif.PTZStaticResponse(operation), true
	}
	return nil, false
}
