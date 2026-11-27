package tcp

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// silentServer accepts TCP connections and never sends a byte,
// so a TLS client handshake against it never completes on its own.
// Accepted connections are delivered to the returned channel.
func silentServer(t *testing.T) (string, <-chan net.Conn) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	conns := make(chan net.Conn, 1)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conns <- conn
		}
	}()

	return ln.Addr().String(), conns
}

func TestDialTLSHandshakeTimeout(t *testing.T) {
	for _, scheme := range []string{"rtspx", "rtsps"} {
		t.Run(scheme, func(t *testing.T) {
			addr, conns := silentServer(t)
			u, err := url.Parse(scheme + "://" + addr + "/stream")
			require.NoError(t, err)

			type result struct {
				conn net.Conn
				err  error
			}
			ch := make(chan result, 1)
			go func() {
				conn, err := Dial(u, 200*time.Millisecond)
				ch <- result{conn, err}
			}()

			select {
			case res := <-ch:
				require.Nil(t, res.conn)
				require.ErrorIs(t, res.err, os.ErrDeadlineExceeded)
			case <-time.After(3 * time.Second):
				t.Fatal("TLS handshake is not bounded by the dial timeout")
			}

			// release the server side of the hanging case
			(<-conns).Close()
		})
	}
}

func TestDialTLSHandshakeFailureClosesConn(t *testing.T) {
	addr, conns := silentServer(t)
	u, err := url.Parse("rtspx://" + addr + "/stream")
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, err := Dial(u, 200*time.Millisecond)
		done <- err
	}()

	server := <-conns
	defer server.Close()

	// the server sees the ClientHello and then EOF once the client closes
	_ = server.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err = io.Copy(io.Discard, server)
	require.NoError(t, err, "connection was not closed after the failed handshake")
	require.Error(t, <-done)
}

func TestDialTLSDeadlineClearedAfterHandshake(t *testing.T) {
	// borrow a self-signed certificate from the test HTTPS server
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	cert := srv.TLS.Certificates[0]
	srv.Close()

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	const timeout = 200 * time.Millisecond

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// finish the handshake, then stay silent for longer than the dial timeout
		_ = conn.(*tls.Conn).Handshake()
		time.Sleep(2 * timeout)
		_, _ = conn.Write([]byte("x"))
	}()

	u, err := url.Parse("rtspx://" + ln.Addr().String() + "/stream")
	require.NoError(t, err)

	conn, err := Dial(u, timeout)
	require.NoError(t, err)
	defer conn.Close()

	// no SetReadDeadline here: it would replace the deadline under test.
	// The server writes after 2*timeout, so a leftover dial deadline fails the read.
	b := make([]byte, 1)
	_, err = io.ReadFull(conn, b)
	require.NoError(t, err)
}
