package rtsp

import (
	"testing"

	pkgrtsp "github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/stretchr/testify/require"
)

func TestApplyClientQueryUsesURLQuery(t *testing.T) {
	conn := &pkgrtsp.Conn{}

	applyClientQuery(conn, "rtsp://127.0.0.1:8554/test?timeout=20&transport=tcp&media=video", "")

	require.Equal(t, 20, conn.Timeout)
	require.Equal(t, "tcp", conn.Transport)
	require.Equal(t, "video", conn.Media)
}

func TestApplyClientQueryRawQueryOverridesURLQuery(t *testing.T) {
	conn := &pkgrtsp.Conn{}

	applyClientQuery(conn, "rtsp://127.0.0.1:8554/test?timeout=20&transport=tcp", "timeout=45#transport=udp#backchannel=1")

	require.Equal(t, 45, conn.Timeout)
	require.Equal(t, "udp", conn.Transport)
	require.True(t, conn.Backchannel)
}

func TestApplyClientQueryAllowsEmptyURL(t *testing.T) {
	conn := &pkgrtsp.Conn{}

	require.NotPanics(t, func() {
		applyClientQuery(conn, "", "")
	})

	require.False(t, conn.Backchannel)
	require.Zero(t, conn.Timeout)
	require.Empty(t, conn.Transport)
	require.Empty(t, conn.Media)
}
