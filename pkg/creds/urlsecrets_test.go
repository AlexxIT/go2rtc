package creds

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddURLSecrets(t *testing.T) {
	// cloud-style source: credentials as query parameters
	src := "nest:?client_id=CLIENT-ID-ABC&client_secret=GOCSPX-verysecretvalue&refresh_token=1//0refreshTOKEN&project_id=proj-123"
	AddURLSecrets(src)

	logLine := "[streams] start producer url=" + src
	got := SecretString(logLine)
	require.NotContains(t, got, "GOCSPX-verysecretvalue")
	require.NotContains(t, got, "1//0refreshTOKEN")
	// identifying detail survives so diagnostics still make sense
	require.Contains(t, got, "client_id=CLIENT-ID-ABC")
	require.Contains(t, got, "project_id=proj-123")
	require.Contains(t, got, "client_secret=***")
	require.Contains(t, got, "refresh_token=***")

	// JSON serialisation of the same URL (what /api/streams emits) is masked too
	require.NotContains(t, SecretString(`{"url":"`+src+`"}`), "GOCSPX-verysecretvalue")
}

func TestAddURLSecretsShortValueIgnored(t *testing.T) {
	// a short value must never become a secret: it would mask innocent text
	// ("key=1234" turning every "1234" in the log into ***)
	AddURLSecrets("roborock://?key=1234&password=ab")
	require.Equal(t, "port 1234 password ab", SecretString("port 1234 password ab"))
}

func TestAddURLSecretsNoQueryOrGarbage(t *testing.T) {
	// no-ops, and no panics
	AddURLSecrets("rtsp://192.168.1.10/stream")
	AddURLSecrets("")
	AddURLSecrets("nest:?%zz=bad&client_secret=stillregisteredvalue")
	// ParseQuery rejects the malformed pair; the rest is still registered
	require.NotContains(t, SecretString("x=stillregisteredvalue"), "stillregisteredvalue")
}
