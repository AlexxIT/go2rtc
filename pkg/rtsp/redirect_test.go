package rtsp

import (
	"bufio"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/stretchr/testify/require"
)

func TestRedirectLoop(t *testing.T) {
	Timeout = time.Second

	ln := listen(t)
	location := "rtsp://" + ln.Addr().String() + "/stream"
	serveRTSP(ln, func(req *tcp.Request) string {
		return "RTSP/1.0 302 Found\r\nLocation: " + location + "\r\n\r\n"
	})

	client := NewClient(location)
	require.Nil(t, client.Dial())
	require.EqualError(t, client.Options(), "rtsp: too many redirects")
}

func TestRedirectAuth(t *testing.T) {
	Timeout = time.Second

	const unauthorized = "RTSP/1.0 401 Unauthorized\r\nWWW-Authenticate: Basic realm=\"test\"\r\n\r\n"

	// other host must never receive credentials
	var otherGotAuth atomic.Bool
	other := listen(t)
	serveRTSP(other, func(req *tcp.Request) string {
		if req.Header.Get("Authorization") != "" {
			otherGotAuth.Store(true)
		}
		return unauthorized
	})

	camera := listen(t)
	serveRTSP(camera, func(req *tcp.Request) string {
		switch req.URL.Path {
		case "/moved":
			return "RTSP/1.0 302 Found\r\nLocation: rtsp://" + other.Addr().String() + "/stream\r\n\r\n"
		case "/relative":
			return "RTSP/1.0 302 Found\r\nLocation: /stream\r\n\r\n"
		}
		if req.Header.Get("Authorization") != "Basic dXNlcjpwYXNz" { // user:pass
			return unauthorized
		}
		return "RTSP/1.0 200 OK\r\n\r\n"
	})

	// same host redirect keeps credentials
	client := NewClient("rtsp://user:pass@" + camera.Addr().String() + "/relative")
	require.Nil(t, client.Dial())
	require.Nil(t, client.Options())

	// other host redirect drops credentials
	client = NewClient("rtsp://user:pass@" + camera.Addr().String() + "/moved")
	require.Nil(t, client.Dial())
	require.EqualError(t, client.Options(), "user/pass not provided")
	require.False(t, otherGotAuth.Load())
}

func listen(t *testing.T) net.Listener {
	ln, err := net.Listen("tcp", "localhost:0")
	require.Nil(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

// serveRTSP answers every RTSP request with the response from handler
func serveRTSP(ln net.Listener, handler func(req *tcp.Request) string) {
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				rd := bufio.NewReader(conn)
				for {
					req, err := tcp.ReadRequest(rd)
					if err != nil {
						return
					}
					if _, err = conn.Write([]byte(handler(req))); err != nil {
						return
					}
				}
			}()
		}
	}()
}
