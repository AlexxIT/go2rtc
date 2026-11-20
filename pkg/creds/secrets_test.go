package creds

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringOverlappingSecrets(t *testing.T) {
	// secrets are process-wide, restore them for other tests
	secretsMu.Lock()
	saved := secrets
	secrets = nil
	secretsReplacer = nil
	secretsMu.Unlock()
	t.Cleanup(func() {
		secretsMu.Lock()
		secrets = saved
		secretsReplacer = nil
		secretsMu.Unlock()
	})

	// the shorter secret is added first, without sorting it would be replaced first
	AddSecret("secret")
	AddSecret("secret-token")

	s := SecretString("http://192.168.1.123/api?key=secret-token")
	require.Equal(t, "http://192.168.1.123/api?key=***", s)
}

func TestString(t *testing.T) {
	AddSecret("admin")
	AddSecret("pa$$word")

	s := SecretString("rtsp://admin:pa$$word@192.168.1.123/stream1")
	require.Equal(t, "rtsp://***:***@192.168.1.123/stream1", s)
}
