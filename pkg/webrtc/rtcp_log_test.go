package webrtc

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/pion/interceptor"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// withLogBuffer routes the package logger into a buffer for one test.
func withLogBuffer(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := log
	SetLogger(zerolog.New(&buf).Level(zerolog.DebugLevel))
	t.Cleanup(func() { log = old })
	return &buf
}

func TestReadMidRTCPLogsErrorBudget(t *testing.T) {
	buf := withLogBuffer(t)

	// the error text could carry an address; it must not reach the log
	failing := func(b []byte) (int, interceptor.Attributes, error) {
		return 0, nil, errors.New("read udp 192.0.2.1:5000: SECRET")
	}
	readMidRTCP("0", failing, func(string) func() { return nil })

	out := buf.String()
	require.Contains(t, out, `"level":"debug"`)
	require.Contains(t, out, `"mid":"0"`)
	require.Contains(t, out, "64 consecutive")
	require.NotContains(t, out, "192.0.2.1")
	require.NotContains(t, out, "SECRET")
}

func TestReadMidRTCPSilentOnClose(t *testing.T) {
	buf := withLogBuffer(t)

	closed := func(b []byte) (int, interceptor.Attributes, error) {
		return 0, nil, io.EOF
	}
	readMidRTCP("0", closed, func(string) func() { return nil })

	require.Empty(t, buf.String())
}
