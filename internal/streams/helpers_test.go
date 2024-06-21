package streams

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactPassword(t *testing.T) {
	require.Equal(t, "not_a_url", RedactPassword("not_a_url"))
	require.Equal(t, "rtsp://localhost:8554", RedactPassword("rtsp://localhost:8554"))
	require.Equal(t, "rtsp://user:xxxxx@localhost:8554", RedactPassword("rtsp://user:password@localhost:8554"))
	require.Equal(t, "rtsp://:xxxxx@localhost:8554", RedactPassword("rtsp://:password@localhost:8554"))
}
