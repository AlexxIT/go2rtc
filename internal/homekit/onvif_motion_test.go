package homekit

import (
	"errors"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/hksv"
	"github.com/rs/zerolog"
)

func TestOnvifMotionWatcherConnectAndPollRenewsBeforeLeaseExpires(t *testing.T) {
	start := time.Unix(0, 0)
	now := start
	stopErr := errors.New("stop pull loop")

	sub := &fakeOnvifPullPoint{
		t:         t,
		now:       &now,
		pullErrAt: 3,
		pullErr:   stopErr,
	}

	w := newOnvifMotionWatcher(&hksv.Server{}, "onvif://camera", 30*time.Second, zerolog.Nop())
	w.now = func() time.Time { return now }
	w.newPullPoint = func(rawURL string, timeout time.Duration) (onvifPullPoint, error) {
		if rawURL != "onvif://camera" {
			t.Fatalf("unexpected ONVIF URL: %s", rawURL)
		}
		if timeout != 60*time.Second {
			t.Fatalf("unexpected subscription timeout: %v", timeout)
		}
		return sub, nil
	}

	err := w.connectAndPoll()
	if !errors.Is(err, stopErr) {
		t.Fatalf("expected %v, got %v", stopErr, err)
	}

	wantPulls := []time.Duration{30 * time.Second, 20 * time.Second, 30 * time.Second}
	if len(sub.pullTimeouts) != len(wantPulls) {
		t.Fatalf("unexpected pull count: got %d want %d", len(sub.pullTimeouts), len(wantPulls))
	}
	for i, want := range wantPulls {
		if sub.pullTimeouts[i] != want {
			t.Fatalf("pull %d timeout mismatch: got %v want %v", i+1, sub.pullTimeouts[i], want)
		}
	}

	if sub.renewCalls != 1 {
		t.Fatalf("expected 1 renew call, got %d", sub.renewCalls)
	}
	if !sub.unsubscribed {
		t.Fatal("expected unsubscribe on exit")
	}
}

type fakeOnvifPullPoint struct {
	t *testing.T

	now *time.Time

	pullTimeouts []time.Duration
	renewCalls   int
	unsubscribed bool

	pullErrAt int
	pullErr   error
}

func (f *fakeOnvifPullPoint) PullMessages(timeout time.Duration, limit int) ([]byte, error) {
	if limit != 10 {
		f.t.Fatalf("unexpected message limit: %d", limit)
	}

	f.pullTimeouts = append(f.pullTimeouts, timeout)
	*f.now = f.now.Add(timeout)

	if f.pullErrAt > 0 && len(f.pullTimeouts) == f.pullErrAt {
		return nil, f.pullErr
	}

	return []byte(`<tev:PullMessagesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl"/>`), nil
}

func (f *fakeOnvifPullPoint) Renew(timeout time.Duration) error {
	if timeout != 60*time.Second {
		f.t.Fatalf("unexpected renew timeout: %v", timeout)
	}

	f.renewCalls++
	return nil
}

func (f *fakeOnvifPullPoint) Unsubscribe() error {
	f.unsubscribed = true
	return nil
}
