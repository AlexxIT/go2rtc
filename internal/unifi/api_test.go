package unifi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

var streamsInit sync.Once

// The api_key of a not yet dialed source must not appear in /api/streams.
func TestAPIStreamsMasksAPIKey(t *testing.T) {
	streamsInit.Do(streams.Init) // registers /api/streams on http.DefaultServeMux

	// unique per run, because creds secrets are process-global (go test -count)
	apiKey := "api-streams-key-" + strconv.Itoa(int(core.NewID()))
	name := "doorbell_" + apiKey

	// a stream from the config exists before the module starts,
	// streams.New only accepts known schemes, so register a placeholder first
	streams.HandleFunc("unifi", func(string) (core.Producer, error) { return nil, nil })
	_, err := streams.New(name, "unifi:https://192.168.1.1?camera_id=camera1&api_key="+apiKey)
	require.NoError(t, err)

	Init()

	for _, path := range []string{"/api/streams", "/api/streams?src=" + name} {
		w := httptest.NewRecorder()
		http.DefaultServeMux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "api_key=***", path)
		require.NotContains(t, w.Body.String(), apiKey, path)
	}
}
