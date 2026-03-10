package webrtc

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/stretchr/testify/require"
)

func TestSyncHandlerReadOnly(t *testing.T) {
	prevReadOnly := api.ReadOnly
	t.Cleanup(func() {
		api.ReadOnly = prevReadOnly
	})

	api.ReadOnly = true

	for _, method := range []string{"POST", "PATCH", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/webrtc?dst=test", nil)
			w := httptest.NewRecorder()

			syncHandler(w, req)

			require.Equal(t, http.StatusForbidden, w.Code)
		})
	}
}
