package ffmpeg

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/stretchr/testify/require"
)

func TestApiFFmpegReadOnly(t *testing.T) {
	prevReadOnly := api.ReadOnly
	t.Cleanup(func() {
		api.ReadOnly = prevReadOnly
	})

	api.ReadOnly = true

	req := httptest.NewRequest("POST", "/api/ffmpeg?dst=cam&text=hello", nil)
	w := httptest.NewRecorder()

	apiFFmpeg(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}
