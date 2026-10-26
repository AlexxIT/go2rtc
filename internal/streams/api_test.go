package streams

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestApiSchemes(t *testing.T) {
	// Setup: Register some test handlers and redirects
	HandleFunc("rtsp", func(url string) (core.Producer, error) { return nil, nil })
	HandleFunc("rtmp", func(url string) (core.Producer, error) { return nil, nil })
	RedirectFunc("http", func(url string) (string, error) { return "", nil })

	t.Run("GET request returns schemes", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/schemes", nil)
		w := httptest.NewRecorder()

		apiSchemes(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var schemes []string
		err := json.Unmarshal(w.Body.Bytes(), &schemes)
		require.NoError(t, err)
		require.NotEmpty(t, schemes)

		// Check that our test schemes are in the response
		require.Contains(t, schemes, "rtsp")
		require.Contains(t, schemes, "rtmp")
		require.Contains(t, schemes, "http")
	})
}

func TestApiSchemesNoDuplicates(t *testing.T) {
	// Setup: Register a scheme in both handlers and redirects
	HandleFunc("duplicate", func(url string) (core.Producer, error) { return nil, nil })
	RedirectFunc("duplicate", func(url string) (string, error) { return "", nil })

	req := httptest.NewRequest("GET", "/api/schemes", nil)
	w := httptest.NewRecorder()

	apiSchemes(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var schemes []string
	err := json.Unmarshal(w.Body.Bytes(), &schemes)
	require.NoError(t, err)

	// Count occurrences of "duplicate"
	count := 0
	for _, scheme := range schemes {
		if scheme == "duplicate" {
			count++
		}
	}

	// Should only appear once
	require.Equal(t, 1, count, "scheme 'duplicate' should appear exactly once")
}

// testConn implements both core.Consumer and core.Producer
type testConn struct {
	core.Connection
	stopped bool
}

func (c *testConn) AddTrack(*core.Media, *core.Codec, *core.Receiver) error { return nil }
func (c *testConn) Start() error                                            { return nil }
func (c *testConn) Stop() error {
	c.stopped = true
	return nil
}

func deleteConnection(rawQuery string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("DELETE", "/api/streams/connection?"+rawQuery, nil)
	w := httptest.NewRecorder()
	apiStreamsConnection(w, req)
	return w
}

func TestApiStreamsConnectionConsumer(t *testing.T) {
	cons := &testConn{Connection: core.Connection{ID: 123}}
	streams["test_cons"] = &Stream{consumers: []core.Consumer{cons}}

	w := deleteConnection("src=test_cons&id=123")

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, cons.stopped)
	require.Empty(t, streams["test_cons"].consumers)
}

func TestApiStreamsConnectionExternalProducer(t *testing.T) {
	prod := &testConn{Connection: core.Connection{ID: 456}}
	stream := &Stream{}
	stream.AddProducer(prod)
	streams["test_prod"] = stream

	w := deleteConnection("src=test_prod&id=456")

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, prod.stopped)
	require.Empty(t, stream.producers)
}

func TestApiStreamsConnectionSkipConfigProducer(t *testing.T) {
	// producer from YAML config must not be stopped by this API
	prod := &testConn{Connection: core.Connection{ID: 789}}
	streams["test_config"] = &Stream{
		producers: []*Producer{{url: "rtsp://camera", conn: prod}},
	}

	w := deleteConnection("src=test_config&id=789")

	require.Equal(t, http.StatusNotFound, w.Code)
	require.False(t, prod.stopped)
}

func TestApiStreamsConnectionErrors(t *testing.T) {
	streams["test_err"] = &Stream{}

	require.Equal(t, http.StatusBadRequest, deleteConnection("src=test_err").Code)
	require.Equal(t, http.StatusBadRequest, deleteConnection("src=test_err&id=abc").Code)
	require.Equal(t, http.StatusNotFound, deleteConnection("src=unknown&id=1").Code)
	require.Equal(t, http.StatusNotFound, deleteConnection("src=test_err&id=1").Code)

	req := httptest.NewRequest("GET", "/api/streams/connection?src=test_err&id=1", nil)
	w := httptest.NewRecorder()
	apiStreamsConnection(w, req)
	require.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
