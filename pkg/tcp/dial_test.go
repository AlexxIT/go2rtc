package tcp

import (
	"context"
	"net"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// silentServer accepts TCP connections and never sends a byte,
// so a TLS client handshake against it never completes on its own.
func silentServer(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var mu sync.Mutex
	var conns []net.Conn
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()

	t.Cleanup(func() {
		_ = ln.Close()
		<-done
		mu.Lock()
		for _, conn := range conns {
			_ = conn.Close() // also ends a client handshake that has no deadline
		}
		mu.Unlock()
	})

	return ln.Addr().String()
}

func TestDialTLSHandshakeTimeout(t *testing.T) {
	for _, scheme := range []string{"rtspx", "rtsps"} {
		t.Run(scheme, func(t *testing.T) {
			u, err := url.Parse(scheme + "://" + silentServer(t) + "/stream")
			require.NoError(t, err)

			const timeout = 200 * time.Millisecond

			type result struct {
				conn net.Conn
				err  error
				took time.Duration
			}
			ch := make(chan result, 1)
			go func() {
				start := time.Now()
				conn, err := Dial(u, timeout)
				ch <- result{conn, err, time.Since(start)}
			}()

			select {
			case res := <-ch:
				require.Nil(t, res.conn)
				require.ErrorIs(t, res.err, os.ErrDeadlineExceeded)
				require.Less(t, res.took, timeout+500*time.Millisecond)
			case <-time.After(2 * time.Second):
				t.Fatal("TLS handshake is not bounded by the dial timeout")
			}
		})
	}
}

func TestDialContextCancelAbortsTLSHandshake(t *testing.T) {
	u, err := url.Parse("rtspx://" + silentServer(t) + "/stream")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	start := time.Now()
	conn, err := DialContext(ctx, u, 5*time.Second)
	took := time.Since(start)

	require.Nil(t, conn)
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, took, time.Second, "handshake not aborted by the context")
}

func TestDialContextCancelled(t *testing.T) {
	u, err := url.Parse("rtsp://" + silentServer(t) + "/stream")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	conn, err := DialContext(ctx, u, 5*time.Second)
	require.Nil(t, conn)
	require.ErrorIs(t, err, context.Canceled)
}
