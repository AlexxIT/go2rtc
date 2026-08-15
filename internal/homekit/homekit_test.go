package homekit

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/creds"
	"github.com/stretchr/testify/require"
)

func TestMaskClientPrivate(t *testing.T) {
	const key = "f939d73d4147d82d243b21c343844ecf787508a41da684ce40fc4b5fd837ab93"
	const rawURL = "homekit://192.168.1.104:34737?device_id=93:92:E5:A4:ED:F8" +
		"&device_public=7e57eefed8282925c908c9806419c02b6914254867b265edbbb4c008" +
		"&client_id=c13bf795-6e9b-b5ab-94f3-26cdf02174bb" +
		"&client_private=" + key

	// not registered yet, so it passes through untouched
	require.Contains(t, creds.SecretString(rawURL), key)

	maskClientPrivate(rawURL)

	masked := creds.SecretString(rawURL)
	require.NotContains(t, masked, key, "private key still present after masking")
	require.Contains(t, masked, "client_private=***")

	// identifiers and the accessory public key are not secrets and must survive,
	// otherwise the API output becomes useless for diagnosing a stream
	require.Contains(t, masked, "device_id=93:92:E5:A4:ED:F8")
	require.Contains(t, masked, "client_id=c13bf795-6e9b-b5ab-94f3-26cdf02174bb")
	require.Contains(t, masked, "device_public=7e57eefed8282925c908c9806419c02b6914254867b265edbbb4c008")
}

func TestMaskClientPrivateIgnoresJunk(t *testing.T) {
	// must not panic or register an empty secret, which would mask everything
	for _, s := range []string{
		"",
		"homekit://192.168.1.104:34737",
		"homekit://192.168.1.104:34737?client_private=",
		"://nonsense",
	} {
		maskClientPrivate(s)
	}

	const probe = "nothing-secret-here"
	require.Equal(t, probe, creds.SecretString(probe))
}
