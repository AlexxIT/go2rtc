package nest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newExtendServer returns an SDM stub whose executeCommand handler is driven
// by the given function: it returns the new expiresAt for a success, or a
// zero time for a 500.
func newExtendServer(t *testing.T, handle func(call int32) time.Time) (*httptest.Server, *atomic.Int32) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		expires := handle(n)
		if expires.IsZero() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		var resv struct {
			Results struct {
				ExpiresAt      time.Time `json:"expiresAt"`
				MediaSessionID string    `json:"mediaSessionId"`
			} `json:"results"`
		}
		resv.Results.ExpiresAt = expires
		resv.Results.MediaSessionID = "session"
		_ = json.NewEncoder(w).Encode(resv)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func setupExtendTest(t *testing.T, srv *httptest.Server) *API {
	oldURL, oldDelays := sdmURL, extendRetryDelays
	sdmURL = srv.URL + "/"
	extendRetryDelays = []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}
	t.Cleanup(func() { sdmURL, extendRetryDelays = oldURL, oldDelays })

	return &API{
		Token:           "token",
		ExpiresAt:       time.Now().Add(time.Hour), // never refresh in tests
		StreamProjectID: "project",
		StreamDeviceID:  "device",
		StreamSessionID: "session",
	}
}

func TestExtendLoopRetriesThenSucceeds(t *testing.T) {
	// The first two extends fail; the third succeeds with a far-away expiry.
	srv, calls := newExtendServer(t, func(call int32) time.Time {
		if call < 3 {
			return time.Time{}
		}
		return time.Now().Add(time.Hour)
	})
	api := setupExtendTest(t, srv)
	// Session expires in 100ms, so the first extend fires immediately
	// (expiry - 1 minute is in the past) and retries fit before expiry.
	api.StreamExpiresAt = time.Now().Add(100 * time.Millisecond)

	lost := make(chan error, 1)
	api.OnSessionLost = func(err error) { lost <- err }

	api.StartExtendStreamTimer()
	defer api.StopExtendStreamTimer()

	deadline := time.After(2 * time.Second)
	for calls.Load() < 3 {
		select {
		case err := <-lost:
			t.Fatalf("session reported lost while retries should succeed: %v", err)
		case <-deadline:
			t.Fatalf("expected 3 extend calls, got %d", calls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}

	if got := api.StreamExpiresAt; time.Until(got) < 30*time.Minute {
		t.Fatalf("StreamExpiresAt not updated after successful extend: %v", got)
	}

	select {
	case err := <-lost:
		t.Fatalf("unexpected OnSessionLost: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestExtendLoopReportsSessionLost(t *testing.T) {
	// Every extend fails; once the session has expired OnSessionLost fires
	// exactly once and the loop exits.
	srv, calls := newExtendServer(t, func(int32) time.Time { return time.Time{} })
	api := setupExtendTest(t, srv)
	api.StreamExpiresAt = time.Now().Add(50 * time.Millisecond)

	var lostCount atomic.Int32
	lost := make(chan error, 1)
	api.OnSessionLost = func(err error) {
		lostCount.Add(1)
		lost <- err
	}

	api.StartExtendStreamTimer()
	defer api.StopExtendStreamTimer()

	select {
	case err := <-lost:
		if err == nil {
			t.Fatal("OnSessionLost called with nil error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnSessionLost was not called")
	}

	if calls.Load() < 2 {
		t.Fatalf("expected at least one retry before giving up, got %d calls", calls.Load())
	}

	// Loop must have exited: no more calls, no second callback.
	n := calls.Load()
	time.Sleep(100 * time.Millisecond)
	if calls.Load() != n {
		t.Fatalf("extend loop kept running after session lost: %d -> %d", n, calls.Load())
	}
	if lostCount.Load() != 1 {
		t.Fatalf("OnSessionLost called %d times", lostCount.Load())
	}
}

func TestStopExtendStreamTimerIsIdempotent(t *testing.T) {
	srv, _ := newExtendServer(t, func(int32) time.Time { return time.Now().Add(time.Hour) })
	api := setupExtendTest(t, srv)
	api.StreamExpiresAt = time.Now().Add(time.Hour)

	api.StartExtendStreamTimer()
	api.StopExtendStreamTimer()
	api.StopExtendStreamTimer() // must not panic on double close
}
