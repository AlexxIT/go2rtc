package nest

import (
	"errors"
	"testing"
	"time"
)

func TestDefinitiveCacheBacksOffExponentially(t *testing.T) {
	now := time.Date(2026, 9, 13, 13, 0, 0, 0, time.UTC)
	c := &definitiveCache{entries: map[string]definitiveError{}, now: func() time.Time { return now }}
	off := &StatusError{Code: 400, Msg: "nest: wrong status: 400 Bad Request: FAILED_PRECONDITION"}

	if c.recent("cam") != nil {
		t.Fatal("nothing cached yet")
	}
	if ttl := c.remember("cam", off); ttl != time.Minute {
		t.Fatalf("first hold-off should be 1m, got %s", ttl)
	}
	if !errors.Is(c.recent("cam"), off) {
		t.Fatal("verdict should be served from cache")
	}

	// the cache expires, Google is asked again and says the same thing
	now = now.Add(61 * time.Second)
	if c.recent("cam") != nil {
		t.Fatal("expired verdict must not be served")
	}
	for i, want := range []time.Duration{2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute, 5 * time.Minute} {
		if ttl := c.remember("cam", off); ttl != want {
			t.Fatalf("repeat %d: hold-off should be %s, got %s", i+1, want, ttl)
		}
		now = now.Add(want + time.Second)
	}

	// a successful dial resets the back-off
	c.forget("cam")
	if ttl := c.remember("cam", off); ttl != time.Minute {
		t.Fatalf("after success the hold-off should restart at 1m, got %s", ttl)
	}

	// other devices are independent
	if ttl := c.remember("other", off); ttl != time.Minute {
		t.Fatalf("other device should start at 1m, got %s", ttl)
	}
}

func TestOnlyDeviceVerdictsAreCached(t *testing.T) {
	cases := map[error]bool{
		&StatusError{Code: 400}:     true,
		&StatusError{Code: 404}:     true,
		&StatusError{Code: 429}:     false,
		&StatusError{Code: 503}:     false,
		&ThrottledError{Wait: 5}:    false,
		errors.New("dial tcp: eof"): false,
	}
	for err, want := range cases {
		if got := isDeviceAnswer(err); got != want {
			t.Errorf("isDeviceAnswer(%v) = %v, want %v", err, got, want)
		}
	}
}
