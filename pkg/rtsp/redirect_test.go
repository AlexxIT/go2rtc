package rtsp

import (
	"bufio"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/stretchr/testify/require"
)

const redirectSDP = "v=0\r\n" +
	"o=- 1 1 IN IP4 0.0.0.0\r\n" +
	"s=-\r\n" +
	"t=0 0\r\n" +
	"m=video 0 RTP/AVP 96\r\n" +
	"a=rtpmap:96 H264/90000\r\n"

// redirectServer answers every request with the response from reply and
// counts accepted connections.
func redirectServer(t *testing.T, reply func(addr, cseq string) string) (addr string, conns *atomic.Int32) {
	t.Helper()

	ln, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	addr = ln.Addr().String()
	conns = new(atomic.Int32)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conns.Add(1)
			go func() {
				defer conn.Close()
				r := bufio.NewReader(conn)
				for {
					req, err := tcp.ReadRequest(r)
					if err != nil {
						return
					}
					res := reply(addr, req.Header.Get("CSeq"))
					if _, err = conn.Write([]byte(res)); err != nil {
						return
					}
				}
			}()
		}
	}()

	return
}

func setTimeout(t *testing.T, d time.Duration) {
	// other tests in this package leave a tiny global Timeout behind
	old := Timeout
	Timeout = d
	t.Cleanup(func() { Timeout = old })
}

func TestRedirectLoop(t *testing.T) {
	setTimeout(t, 5*time.Second)

	addr, conns := redirectServer(t, func(addr, cseq string) string {
		return "RTSP/1.0 302 Found\r\nCSeq: " + cseq + "\r\nLocation: rtsp://" + addr + "/stream\r\n\r\n"
	})

	client := NewClient("rtsp://" + addr + "/stream")
	require.NoError(t, client.Dial())

	ch := make(chan error, 1)
	go func() { ch <- client.Describe() }()

	select {
	case err := <-ch:
		require.ErrorContains(t, err, "too many redirects")
	case <-time.After(5 * time.Second):
		t.Fatal("redirect loop is not limited")
	}

	// the first connection plus maxRedirects followed redirects
	require.Equal(t, int32(maxRedirects+1), conns.Load())
}

func TestRedirectOnce(t *testing.T) {
	setTimeout(t, 5*time.Second)

	target, _ := redirectServer(t, func(_, cseq string) string {
		return "RTSP/1.0 200 OK\r\nCSeq: " + cseq + "\r\nContent-Type: application/sdp\r\nContent-Length: " +
			strconv.Itoa(len(redirectSDP)) + "\r\n\r\n" + redirectSDP
	})
	source, _ := redirectServer(t, func(_, cseq string) string {
		return "RTSP/1.0 302 Found\r\nCSeq: " + cseq + "\r\nLocation: rtsp://" + target + "/other\r\n\r\n"
	})

	client := NewClient("rtsp://" + source + "/stream")
	require.NoError(t, client.Dial())
	require.NoError(t, client.Describe())
	require.Len(t, client.Medias, 1)
	require.Equal(t, target, client.URL.Host)
}
