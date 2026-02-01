package streams

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/stretchr/testify/require"
)

func TestApiStreamsReadOnly(t *testing.T) {
	prevReadOnly := api.ReadOnly
	t.Cleanup(func() {
		api.ReadOnly = prevReadOnly
	})

	api.ReadOnly = true

	for _, method := range []string{"PUT", "PATCH", "POST", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/streams?src=test", nil)
			w := httptest.NewRecorder()

			apiStreams(w, req)

			require.Equal(t, http.StatusForbidden, w.Code)
		})
	}

	t.Run("GET allowed", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/streams", nil)
		w := httptest.NewRecorder()

		apiStreams(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestApiPreloadReadOnly(t *testing.T) {
	prevReadOnly := api.ReadOnly
	t.Cleanup(func() {
		api.ReadOnly = prevReadOnly
	})

	api.ReadOnly = true

	for _, method := range []string{"PUT", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/preload?src=test", nil)
			w := httptest.NewRecorder()

			apiPreload(w, req)

			require.Equal(t, http.StatusForbidden, w.Code)
		})
	}

	t.Run("GET allowed", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/preload", nil)
		w := httptest.NewRecorder()

		apiPreload(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})
}
