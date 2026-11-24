package reolink

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
)

const (
	initialLoginCooldown = 15 * time.Second
	initialDialCooldown  = time.Second
	fastDialWindow       = 3 * time.Minute
	maxCooldown          = 5 * time.Minute
	maxCooldowns         = 256
	loginRefusedStatus   = 407
)

var errLoginCooldown = errors.New("reolink: login paused after camera rejection")
var errDialCooldown = errors.New("reolink: TCP connection retry paused")

type cooldown struct {
	until     time.Time
	fastUntil time.Time
	delay     time.Duration
	probing   bool
}

func (c *cooldown) expired(now time.Time) bool {
	return !c.probing && now.Sub(c.until) >= maxCooldown
}

// The registry lock protects each gate and its outstanding probe.
type cooldowns[K comparable] map[K]*cooldown

func (g cooldowns[K]) begin(key K, now time.Time, blocked error) (*cooldown, error) {
	state := g[key]
	if state == nil {
		return nil, nil
	}
	if state.expired(now) {
		delete(g, key)
		return nil, nil
	}
	if state.probing || now.Before(state.until) {
		return nil, blocked
	}
	state.probing = true
	return state, nil
}

func (g *cooldowns[K]) finish(key K, trial *cooldown, now time.Time, err error, failed bool, initial, fast time.Duration) time.Duration {
	state := (*g)[key]
	switch {
	case trial != nil && trial != state:
		return 0
	case err == nil:
		delete(*g, key)
		return 0
	case trial != nil:
		state.probing = false
	case !failed || state != nil:
		// Failures already in flight share the cooldown set by the first one.
		return 0
	default:
		if len(*g) == maxCooldowns {
			for key, entry := range *g {
				if entry.expired(now) {
					delete(*g, key)
				}
			}
		}
		if len(*g) == maxCooldowns {
			return 0
		}
		if *g == nil {
			*g = make(cooldowns[K])
		}
		state = &cooldown{fastUntil: now.Add(fast)}
		(*g)[key] = state
	}
	if !failed {
		return 0
	}
	delay := initial
	if !now.Before(state.fastUntil) {
		delay = min(max(initial, state.delay*2), maxCooldown)
	}
	state.delay, state.until = delay, now.Add(delay)
	return delay
}

func (r *Registry) beginLogin(key cameraKey, now time.Time) (*cooldown, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.logins.begin(key, now, errLoginCooldown)
}

func (r *Registry) finishLogin(key cameraKey, trial *cooldown, err error, now time.Time) {
	var status *baichuan.StatusError
	rejected := errors.As(err, &status) && status.Code == loginRefusedStatus
	if trial == nil && err != nil && !rejected {
		return
	}
	r.mu.Lock()
	delay := r.logins.finish(key, trial, now, err, rejected, initialLoginCooldown, 0)
	r.mu.Unlock()
	if delay != 0 {
		r.log.Warn().Str("camera", key.endpoint).Dur("retry_in", delay).
			Uint16("status", loginRefusedStatus).Msg("reolink login cooldown")
	}
}

func (r *Registry) dialCamera(ctx context.Context, endpoint string, cfg baichuan.Config) (*baichuan.Client, error) {
	if cfg.Host == "" {
		return baichuan.Dial(ctx, cfg)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	trial, err := r.dials.begin(endpoint, time.Now(), errDialCooldown)
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}
	client, err := baichuan.Dial(ctx, cfg)
	var op *net.OpError
	failed := errors.As(err, &op) && op.Op == "dial" && !errors.Is(err, context.Canceled)
	if trial != nil || failed || err == nil {
		r.mu.Lock()
		delay := r.dials.finish(endpoint, trial, time.Now(), err, failed, initialDialCooldown, fastDialWindow)
		r.mu.Unlock()
		if delay != 0 {
			r.log.Debug().Str("camera", endpoint).Dur("retry_in", delay).Msg("reolink TCP retry scheduled")
		}
	}
	return client, err
}
