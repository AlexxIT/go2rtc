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
	require.Equal(t, "rtsp://***@192.168.1.123/stream1", s)

	s = SecretString("http://192.168.1.123/cgi?user=admin&pwd=pa$$word")
	require.Equal(t, "http://192.168.1.123/cgi?user=***&pwd=***", s)
}

func TestStripUserinfo(t *testing.T) {
	s := `streams:
  test:
    - ffmpeg:rtsp://username:password@10.1.2.3:554/stream1
    - ffmpeg:rtsp://10.1.2.3:554/stream1@#video=copy
`
	s = SecretString(s)
	require.Equal(t, `streams:
  test:
    - ffmpeg:rtsp://***@10.1.2.3:554/stream1
    - ffmpeg:rtsp://10.1.2.3:554/stream1@#video=copy
`, s)
}
