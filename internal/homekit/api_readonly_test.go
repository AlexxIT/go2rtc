package homekit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/stretchr/testify/require"
)

func TestApiHomekitReadOnly(t *testing.T) {
	prevReadOnly := api.ReadOnly
	t.Cleanup(func() {
		api.ReadOnly = prevReadOnly
	})

	api.ReadOnly = true

	t.Run("POST blocked", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/homekit", nil)
		w := httptest.NewRecorder()

		apiHomekit(w, req)

		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("GET allowed", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/homekit", nil)
		w := httptest.NewRecorder()

		apiHomekit(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})
}
