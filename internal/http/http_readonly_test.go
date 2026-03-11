package http

import (
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/stretchr/testify/require"
)

func TestApiStreamReadOnly(t *testing.T) {
	prevReadOnly := api.ReadOnly
	t.Cleanup(func() {
		api.ReadOnly = prevReadOnly
	})

	api.ReadOnly = true

	for _, method := range []string{"PUT", "PATCH", "POST", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/stream?dst=test", nil)
			w := httptest.NewRecorder()

			apiStream(w, req)

			require.Equal(t, stdhttp.StatusForbidden, w.Code)
		})
	}
}
