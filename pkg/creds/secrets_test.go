package creds

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	AddSecret("admin")
	AddSecret("pa$$word")

	s := SecretString("rtsp://admin:pa$$word@192.168.1.123/stream1")
	require.Equal(t, "rtsp://***:***@192.168.1.123/stream1", s)
}

func TestStringOverlappingSecrets(t *testing.T) {
	AddSecret("rtp://127.0.0.1:6500")
	AddSecret("rtp://127.0.0.1:6500/talkback?token=secret")

	s := SecretString(`{"url":"rtp://127.0.0.1:6500/talkback?token=secret"}`)
	require.Equal(t, `{"url":"***"}`, s)
}
