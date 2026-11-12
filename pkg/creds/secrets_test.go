package creds

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	AddSecret("admin")
	AddSecret("pa$$word")

	s := SecretString("rtsp://admin:pa$$word@192.168.1.123/stream1")
	require.Equal(t, "rtsp://***:***@192.168.1.123/stream1", s)
}

type recordWriter struct {
	writes []string
}

func (r *recordWriter) Write(b []byte) (int, error) {
	r.writes = append(r.writes, string(b))
	return len(b), nil
}

func TestSecretWriterSingleWrite(t *testing.T) {
	AddSecret("pa$$word")

	rec := &recordWriter{}
	w := SecretWriter(rec)

	line := `{"level":"info","message":"url rtsp://admin:pa$$word@host/stream token pa$$word end"}` + "\n"
	n, err := w.Write([]byte(line))
	require.NoError(t, err)

	// zerolog expects exactly one complete event per Write call
	require.Equal(t, []string{`{"level":"info","message":"url rtsp://***@host/stream token *** end"}` + "\n"}, rec.writes)
	require.Equal(t, len(line), n)
}

func TestSecretWriterZerolog(t *testing.T) {
	AddSecret("pa$$word")

	var errs []error
	handler := zerolog.ErrorHandler
	zerolog.ErrorHandler = func(err error) { errs = append(errs, err) }
	defer func() { zerolog.ErrorHandler = handler }()

	var out bytes.Buffer
	console := zerolog.ConsoleWriter{Out: &out, NoColor: true}
	logger := zerolog.New(SecretWriter(console))

	logger.Info().Str("token", "pa$$word").Msg("secret pa$$word in message")

	require.Empty(t, errs)
	require.Contains(t, out.String(), "secret *** in message")
	require.Contains(t, out.String(), "token=***")
	require.NotContains(t, out.String(), "pa$$word")
}

func TestSecretWriterReturnsInputLength(t *testing.T) {
	AddSecret("pa$$word")

	// replacement ("***") is shorter than the secret, so len(output) != len(input)
	b := []byte("pa$$word pa$$word")
	n, err := SecretWriter(&recordWriter{}).Write(b)
	require.NoError(t, err)
	require.Equal(t, len(b), n)

	rec := httptest.NewRecorder()
	n, err = SecretResponse(rec).Write(b)
	require.NoError(t, err)
	require.Equal(t, len(b), n)
	require.Equal(t, "*** ***", rec.Body.String())
}
