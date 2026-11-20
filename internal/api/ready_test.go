package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
)

func TestMiddlewareReadyStartup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ready := make(chan struct{})
		signalReady := sync.OnceFunc(func() { close(ready) })
		defer signalReady()
		mux := http.NewServeMux()
		gate := middlewareReady("/api/schemes", ready, mux)
		done := make(chan struct{}, 2)

		early := httptest.NewRecorder()
		go func() {
			gate.ServeHTTP(early, httptest.NewRequest("GET", "/api/schemes", nil))
			done <- struct{}{}
		}()
		synctest.Wait()
		require.Empty(t, done, "request must wait even before route registration")

		schemes := []string{"rtsp"}
		mux.HandleFunc("/api/schemes", func(w http.ResponseWriter, r *http.Request) {
			ResponseJSON(w, schemes)
		})
		late := httptest.NewRecorder()
		go func() {
			gate.ServeHTTP(late, httptest.NewRequest("GET", "/api/schemes", nil))
			done <- struct{}{}
		}()
		synctest.Wait()
		require.Empty(t, done, "registered route must still wait for all schemes")

		schemes = append(schemes, "ffmpeg")
		signalReady()
		synctest.Wait()
		require.Len(t, done, 2, "readiness must release both requests")
		for _, w := range []*httptest.ResponseRecorder{early, late} {
			require.Equal(t, http.StatusOK, w.Code)
			require.JSONEq(t, `["rtsp","ffmpeg"]`, w.Body.String())
		}
	})
}

func TestMiddlewareReadyCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ready := make(chan struct{})
		defer close(ready)
		called := false
		gate := middlewareReady("/api/schemes", ready, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		w := httptest.NewRecorder()
		w.Code = 0 // NewRecorder defaults to 200; zero detects any response write.
		done := make(chan struct{})
		go func() {
			gate.ServeHTTP(w, httptest.NewRequestWithContext(ctx, "GET", "/api/schemes", nil))
			close(done)
		}()
		synctest.Wait()
		cancel()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("canceled request is still waiting for initialization")
		}
		require.False(t, called, "canceled request reached the scheme handler")
		require.Zero(t, w.Code)
		require.Empty(t, w.Body.String())
	})
}
