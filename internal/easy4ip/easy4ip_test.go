package easy4ip

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseURL(t *testing.T) {
	tg, err := parseURL("easy4ip://admin:p%40ss@9A0123456789/cam/realmonitor?channel=2&subtype=1&rtsp_port=8554&http_port=8080&relay=1&client_id=abc:37777#backchannel=0")
	require.NoError(t, err)
	require.Equal(t, "9A0123456789", tg.serial)
	require.Equal(t, "admin", tg.user)
	require.Equal(t, "p@ss", tg.pass)
	require.Equal(t, 8554, tg.rtspPort)
	require.Equal(t, 8080, tg.httpPort)
	require.True(t, tg.relay)
	require.Equal(t, "abc:37777", tg.clientID)

	// our options never reach the NVR, everything else does
	require.Equal(t, "rtsp://admin:p%40ss@127.0.0.1:41000/cam/realmonitor?channel=2&subtype=1#backchannel=0", tg.location(41000))
}

// The NVR needs the query of a playback URL as the client wrote it (channel, subtype, starttime, endtime).
func TestParseURLKeepsQueryOrder(t *testing.T) {
	tg, err := parseURL("easy4ip://admin:pw@SN1/cam/playback?channel=1&subtype=0&relay=1&starttime=2026_10_14_13_55_19&client_id=abc:37777&endtime=2026_10_14_23_59_59")
	require.NoError(t, err)
	require.Equal(t, "rtsp://admin:pw@127.0.0.1:41000/cam/playback?channel=1&subtype=0&starttime=2026_10_14_13_55_19&endtime=2026_10_14_23_59_59", tg.location(41000))
}

func TestParseURLDefaults(t *testing.T) {
	tg, err := parseURL("easy4ip://admin:pw@SN1/cam/realmonitor?channel=1")
	require.NoError(t, err)
	require.Equal(t, 554, tg.rtspPort)
	require.Zero(t, tg.httpPort)
	require.False(t, tg.relay)
	require.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}:37777$`, tg.clientID)

	again, _ := parseURL("easy4ip://other:pw@SN1/x")
	require.Equal(t, tg.clientID, again.clientID, "stable per host and serial")
	other, _ := parseURL("easy4ip://admin:pw@SN2/x")
	require.NotEqual(t, tg.clientID, other.clientID)
}

func TestParseURLErrors(t *testing.T) {
	for _, bad := range []string{"easy4ip://SN1/path", "easy4ip://:pw@/path", "easy4ip://%zz"} {
		_, err := parseURL(bad)
		require.Error(t, err, bad)
	}
}
