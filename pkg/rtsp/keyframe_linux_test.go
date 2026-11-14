//go:build linux

package rtsp

import (
	"errors"
	"net"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// synDropAddr returns the address of a listening socket with backlog 0 that
// never accepts. Once its accept queue is full, the kernel drops further SYNs
// and connect() hangs until the dial timeout.
func synDropAddr(t *testing.T) string {
	t.Helper()

	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = syscall.Close(fd) })
	require.NoError(t, syscall.Bind(fd, &syscall.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}))
	require.NoError(t, syscall.Listen(fd, 0))
	sa, err := syscall.Getsockname(fd)
	require.NoError(t, err)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(sa.(*syscall.SockaddrInet4).Port))

	// fill the accept queue
	for i := 0; i < 4; i++ {
		if conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			t.Cleanup(func() { _ = conn.Close() })
		}
	}

	// the precondition of the test: a further connect really hangs
	_, err = net.DialTimeout("tcp", addr, 300*time.Millisecond)
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Skipf("connect to a full accept queue does not hang here: %v", err)
	}
	return addr
}

func TestStopEndsKeyframeHelperInTCPConnect(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	addr := synDropAddr(t)
	c := newSetupConn(t, srv)
	c.Timeout = 5 // dial timeout of the helper in seconds; set before Start
	startPlaying(t, srv, c)

	// nothing is accepted, so the dial phase is reached once the helper runs
	// for a while without a result
	dialing := make(chan struct{})
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(dialing)
	}()

	assertStopEndsDialingHelper(t, srv, c, "rtsp://user:pass@"+addr+"/SECRETPATHTOKEN", dialing)
}
