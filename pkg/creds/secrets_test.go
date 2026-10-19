package creds

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	AddSecret("admin")
	AddSecret("pa$$word")

	s := SecretString("rtsp://admin:pa$$word@192.168.1.123/stream1")
	// Since "Strip Userinfo from log entries and errors" (#2051) the whole
	// user:pass@ block collapses to ***@ — one marker, no hint of the shape.
	require.Equal(t, "rtsp://***@192.168.1.123/stream1", s)
}
