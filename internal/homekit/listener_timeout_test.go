package homekit

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/stretchr/testify/require"
)

// A pipe exercises net/http's connection handling without binding a socket.
type hapPipeListener struct {
	conn     net.Conn
	accepted bool
	closed   chan struct{}
	once     sync.Once
}

func (l *hapPipeListener) Accept() (net.Conn, error) {
	if !l.accepted {
		l.accepted = true
		return l.conn, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}
func (l *hapPipeListener) Close() error {
	l.once.Do(func() { close(l.closed); _ = l.conn.Close() })
	return nil
}
func (l *hapPipeListener) Addr() net.Addr { return l.conn.LocalAddr() }

type hapTestConn struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

func (c *hapTestConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { close(c.closed) })
	return err
}

func pipeHAPServer(t *testing.T) (net.Conn, <-chan struct{}) {
	t.Helper()
	a, b := net.Pipe()
	conn := &hapTestConn{Conn: a, closed: make(chan struct{})}
	ln := &hapPipeListener{conn: conn, closed: make(chan struct{})}
	srv := newHAPHTTPServer("pipe")
	// Shorten configured limits only; a missing limit must still fail the test.
	if srv.ReadTimeout > 0 {
		srv.ReadTimeout = 80 * time.Millisecond
	}
	if srv.IdleTimeout > 0 {
		srv.IdleTimeout = 80 * time.Millisecond
	}
	srv.ReadHeaderTimeout = 80 * time.Millisecond
	served := make(chan struct{})
	go func() {
		defer close(served)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("HTTP server: %v", err)
		}
	}()
	t.Cleanup(func() {
		_ = b.Close()
		_ = srv.Close()
		select {
		case <-conn.closed:
		case <-time.After(time.Second):
			t.Error("server connection did not close")
		}
		<-served
	})
	require.NoError(t, b.SetDeadline(time.Now().Add(2*time.Second)))
	return b, conn.closed
}

func TestHAPRejectedRequestBodyTimeout(t *testing.T) {
	for _, line := range []string{"POST /unknown", "GET /pair-verify"} {
		t.Run(line, func(t *testing.T) {
			conn, closed := pipeHAPServer(t)
			_, err := io.WriteString(conn, line+" HTTP/1.1\r\nHost: test\r\nContent-Length: 100\r\n\r\n")
			require.NoError(t, err)
			_, err = io.ReadAll(conn)
			require.NoError(t, err, "rejected request must close without waiting indefinitely for its body")
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("rejected connection remained open")
			}
		})
	}
}

func TestHAPHTTPIdleTimeout(t *testing.T) {
	conn, _ := pipeHAPServer(t)
	_, err := io.WriteString(conn, "GET /unknown HTTP/1.1\r\nHost: test\r\n\r\n")
	require.NoError(t, err)
	res, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, res.StatusCode)
	require.False(t, res.Close, "test requires an ordinary keep-alive response")
	_, err = io.Copy(io.Discard, res.Body)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	var data [1]byte
	_, err = conn.Read(data[:])
	require.ErrorIs(t, err, io.EOF, "idle HTTP connection must close before the client deadline")
}

func TestHAPVerifiedSessionOutlivesHTTPTimeouts(t *testing.T) {
	s := testCamera(t)
	controller := newController("pipe", s)
	controller.DevicePublic = s.hap.ServerPublic()
	s.AddPair(controller.ClientID, controller.ClientPublic(), hap.PermissionAdmin)
	conn, closed := pipeHAPServer(t)
	secure := verifyConnection(t, conn, "pipe", controller, s, true)
	time.Sleep(200 * time.Millisecond)
	req, err := http.NewRequest("GET", "http://pipe"+hap.PathAccessories, nil)
	require.NoError(t, err)
	require.NoError(t, req.Write(secure))
	res, err := http.ReadResponse(bufio.NewReader(secure), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	_, err = io.Copy(io.Discard, res.Body)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.NoError(t, secure.Close())
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("HAP handler did not finish")
	}
}
