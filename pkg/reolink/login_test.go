package reolink

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
)

func TestLoginCooldownAdmitsOneRetry(t *testing.T) {
	registry := testRegistry(nil)
	key := cameraKey{endpoint: "cooldown-camera"}
	now := time.Unix(0, 0)
	refused := fmt.Errorf("login: %w", &baichuan.StatusError{Command: 1, Code: 407})
	registry.finishLogin(key, nil, refused, now)
	registry.finishLogin(key, nil, refused, now.Add(time.Second))
	camera := &Camera{registry: registry, key: cameraKey{endpoint: "refetch-camera"}}
	registry.finishLogin(camera.key, nil, refused, time.Now())
	// Exercise the same admission gate used by both ordinary and history dials.
	if _, err := openBaichuan(context.Background(), camera, source{preRecord: 1}); err != errLoginCooldown {
		t.Fatalf("history did not respect cooldown: %v", err)
	}
	if _, err := registry.beginLogin(cameraKey{endpoint: "other-camera"}, now); err != nil {
		t.Fatal(err)
	}

	now = now.Add(initialLoginCooldown)
	retries := make(chan *cooldown, 16)
	for range cap(retries) {
		go func() {
			trial, _ := registry.beginLogin(key, now)
			retries <- trial
		}()
	}
	var accepted *cooldown
	for range cap(retries) {
		if trial := <-retries; trial != nil {
			if accepted != nil {
				t.Fatal("multiple retries crossed the same cooldown")
			}
			accepted = trial
		}
	}
	if accepted == nil {
		t.Fatal("no retry admitted at the end of the initial cooldown")
	}
	registry.finishLogin(key, accepted, refused, now)
	for delay := initialLoginCooldown * 2; ; delay = min(delay*2, maxCooldown) {
		if _, err := registry.beginLogin(key, now.Add(delay-time.Nanosecond)); !errors.Is(err, errLoginCooldown) {
			t.Fatalf("retry before %s: %v", delay, err)
		}
		now = now.Add(delay)
		retry, err := registry.beginLogin(key, now)
		if err != nil || retry == nil {
			t.Fatalf("retry after %s: %v", delay, err)
		}
		if delay == maxCooldown {
			registry.finishLogin(key, retry, context.DeadlineExceeded, now)
			retry, err = registry.beginLogin(key, now)
			if err != nil || retry == nil {
				t.Fatalf("network error increased cooldown: %v", err)
			}
			registry.finishLogin(key, retry, nil, now)
			break
		}
		registry.finishLogin(key, retry, refused, now)
	}
	if trial, err := registry.beginLogin(key, now); trial != nil || err != nil {
		t.Fatalf("successful login retained cooldown: %v", err)
	}
}

func TestLoginCooldownBoundsFailureRecords(t *testing.T) {
	r := testRegistry(nil)
	now := time.Unix(0, 0)
	first := cameraKey{endpoint: "first"}
	r.finishLogin(first, nil, context.DeadlineExceeded, now)
	r.finishLogin(first, nil, &baichuan.StatusError{Command: 1, Code: 400}, now)
	if len(r.logins) != 0 {
		t.Fatal("unclassified errors created a cooldown")
	}
	refused := &baichuan.StatusError{Command: 1, Code: 407}
	for i := range maxCooldowns + 2 {
		r.finishLogin(cameraKey{endpoint: fmt.Sprint(i)}, nil, refused, now)
	}
	if len(r.logins) != maxCooldowns {
		t.Fatalf("failure records: %d", len(r.logins))
	}
	r.finishLogin(first, nil, refused, now.Add(initialLoginCooldown+maxCooldown))
	if len(r.logins) != 1 || r.logins[first] == nil {
		t.Fatal("expired failures were not reclaimed")
	}
}

func TestLoginCooldownClearsOnLateInitialSuccess(t *testing.T) {
	r := testRegistry(nil)
	key := cameraKey{endpoint: "camera"}
	now := time.Unix(0, 0)
	initial, err := r.beginLogin(key, now)
	if initial != nil || err != nil {
		t.Fatalf("initial login gated: %v", err)
	}
	refused := &baichuan.StatusError{Command: 1, Code: 407}
	r.finishLogin(key, nil, refused, now)
	now = now.Add(initialLoginCooldown)
	probe, err := r.beginLogin(key, now)
	if probe == nil || err != nil {
		t.Fatalf("retry not admitted: %v", err)
	}
	r.finishLogin(key, initial, nil, now)
	if len(r.logins) != 0 {
		t.Fatal("successful initial login left the camera blocked")
	}
	r.finishLogin(key, nil, refused, now)
	r.finishLogin(key, probe, nil, now)
	if state := r.logins[key]; state == nil || state == probe || state.until != now.Add(initialLoginCooldown) {
		t.Fatal("old probe affected a newer cooldown")
	}
}

func TestDialCooldownSchedule(t *testing.T) {
	var dials cooldowns[string]
	endpoint := "offline-camera:9000"
	now := time.Unix(0, 0)
	failure := &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}
	finish := func(trial *cooldown, when time.Time) time.Duration {
		return dials.finish(endpoint, trial, when, failure, true, initialDialCooldown, fastDialWindow)
	}
	if delay := finish(nil, now); delay != time.Second {
		t.Fatalf("initial delay: %v", delay)
	}
	if delay := finish(nil, now); delay != 0 {
		t.Fatalf("simultaneous failure escalated delay: %v", delay)
	}
	for waited := time.Second; waited < fastDialWindow; waited += time.Second {
		now = now.Add(time.Second)
		trial, err := dials.begin(endpoint, now, errDialCooldown)
		if err != nil || trial == nil || finish(trial, now) != time.Second {
			t.Fatalf("fast retry at %v: %v", waited, err)
		}
	}
	for delay := time.Second; ; delay = min(delay*2, maxCooldown) {
		now = now.Add(delay)
		trial, err := dials.begin(endpoint, now, errDialCooldown)
		if err != nil || trial == nil {
			t.Fatalf("probe after %v: %v", delay, err)
		}
		if _, err := dials.begin(endpoint, now, errDialCooldown); err != errDialCooldown {
			t.Fatalf("second concurrent probe: %v", err)
		}
		if next := finish(trial, now); next != min(delay*2, maxCooldown) {
			t.Fatalf("backoff after %v: %v", delay, next)
		}
		if _, err := dials.begin(endpoint, dials[endpoint].until.Add(-time.Nanosecond), errDialCooldown); err != errDialCooldown {
			t.Fatalf("probe before deadline: %v", err)
		}
		if delay == maxCooldown {
			break
		}
	}
}

func TestDialCooldownIsSharedAndClearsOnConnect(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().(*net.TCPAddr)
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	registry := testRegistry(nil)
	endpoint := address.String()
	cfg := baichuan.NewConfig(address.IP.String(), "user", "password")
	cfg.Port = uint16(address.Port)
	canceled, abort := context.WithCancel(context.Background())
	abort()
	if _, err := registry.dialCamera(canceled, endpoint, cfg); !errors.Is(err, context.Canceled) || len(registry.dials) != 0 {
		t.Fatalf("canceled dial affected backoff: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := registry.dialCamera(ctx, endpoint, cfg)
	if client != nil {
		client.Close()
		t.Fatal("unexpected connection to closed listener")
	}
	if err == nil || len(registry.dials) != 1 {
		t.Fatalf("failed connection did not create backoff: %v", err)
	}
	now := time.Now()
	registry.dials[endpoint].until = now.Add(time.Minute)
	cfg = baichuan.NewConfig(address.IP.String(), "alternate-user", "password")
	cfg.Port = uint16(address.Port)
	if _, err := registry.dialCamera(ctx, endpoint, cfg); err != errDialCooldown || errorClass(err) != "dial_cooldown" {
		t.Fatalf("connection crossed its endpoint's backoff: %v", err)
	}
	if _, err := registry.dials.begin("other-camera:9000", now, errDialCooldown); err != nil {
		t.Fatalf("backoff blocked another endpoint: %v", err)
	}
	listener, err = net.Listen("tcp4", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	registry.dials[endpoint].until = now
	client, err = registry.dialCamera(ctx, endpoint, cfg)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	if len(registry.dials) != 0 {
		t.Fatal("successful TCP connection retained endpoint backoff")
	}
	if len(registry.logins) != 0 {
		t.Fatal("TCP retries changed credential cooldowns")
	}
}
