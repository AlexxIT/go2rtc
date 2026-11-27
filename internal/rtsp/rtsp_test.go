package rtsp

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublishAllowed(t *testing.T) {
	defer func() { allowPublish = nil }()

	addr := func(s string) net.Addr {
		return &net.TCPAddr{IP: net.ParseIP(s), Port: 50000}
	}

	allowPublish = nil // default - anyone
	require.True(t, publishAllowed(addr("192.168.1.50")))

	allowPublish = parseNetworks([]string{}) // only localhost
	require.True(t, publishAllowed(addr("127.0.0.1")))
	require.True(t, publishAllowed(addr("::1")))
	require.False(t, publishAllowed(addr("192.168.1.50")))

	allowPublish = parseNetworks([]string{"192.168.1.0/24", "10.0.0.5", "fd00::/8", "bad"})
	require.Len(t, allowPublish, 3)
	require.True(t, publishAllowed(addr("192.168.1.50")))
	require.True(t, publishAllowed(addr("::ffff:192.168.1.50")))
	require.True(t, publishAllowed(addr("10.0.0.5")))
	require.False(t, publishAllowed(addr("10.0.0.6")))
	require.True(t, publishAllowed(addr("fd00::1")))
	require.False(t, publishAllowed(addr("192.168.2.1")))
}
