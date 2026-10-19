package xnet

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListen(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	// the port is busy, Listen should wait for it
	time.AfterFunc(time.Millisecond*300, func() { _ = ln.Close() })

	ln2, err := Listen("tcp", ln.Addr().String())
	require.NoError(t, err)
	require.NoError(t, ln2.Close())
}
