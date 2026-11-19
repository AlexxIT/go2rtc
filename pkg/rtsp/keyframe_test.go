package rtsp

import (
	"bufio"
	"math"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

// keyframeCamera records the methods of all sessions
type keyframeCamera struct {
	uri      string
	redirect string // answer DESCRIBE with a redirect to this URI

	mu      sync.Mutex
	methods []string
	closed  chan struct{} // one value per closed connection
}

func newKeyframeCamera(t *testing.T) *keyframeCamera {
	ln, err := net.Listen("tcp", "localhost:0")
	require.Nil(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	c := &keyframeCamera{uri: "rtsp://" + ln.Addr().String() + "/stream", closed: make(chan struct{}, 10)}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go c.serve(conn)
		}
	}()

	return c
}

func (c *keyframeCamera) serve(conn net.Conn) {
	defer func() {
		_ = conn.Close()
		c.closed <- struct{}{}
	}()

	r := textproto.NewReader(bufio.NewReader(conn))
	for {
		line, err := r.ReadLine() // ex. "DESCRIBE rtsp://host/stream RTSP/1.0"
		if err != nil {
			return
		}
		header, _ := r.ReadMIMEHeader()

		method, _, _ := strings.Cut(line, " ")
		c.mu.Lock()
		c.methods = append(c.methods, method)
		c.mu.Unlock()

		res := "RTSP/1.0 200 OK\r\nCseq: " + header.Get("Cseq") + "\r\n"
		switch method {
		case MethodDescribe:
			if c.redirect != "" {
				res = "RTSP/1.0 302 Found\r\nCseq: " + header.Get("Cseq") + "\r\nLocation: " + c.redirect + "\r\n\r\n"
				break
			}
			sdp := "v=0\r\no=- 1 1 IN IP4 0.0.0.0\r\ns=-\r\nt=0 0\r\n" +
				"m=audio 0 RTP/AVP 8\r\na=rtpmap:8 PCMA/8000\r\n" +
				"m=video 0 RTP/AVP 96\r\na=rtpmap:96 H264/90000\r\n"
			res += "Content-Type: application/sdp\r\nContent-Length: " + strconv.Itoa(len(sdp)) + "\r\n\r\n" + sdp
		case MethodSetup:
			res += "Transport: RTP/AVP/TCP;unicast;interleaved=2-3\r\nSession: 1\r\n\r\n"
		case MethodTeardown:
			continue // the client closes the connection without waiting for a response
		default:
			res += "\r\n"
		}
		_, _ = conn.Write([]byte(res))
	}
}

func (c *keyframeCamera) sessions() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.methods...)
}

func (c *keyframeCamera) waitClosed(t *testing.T) {
	select {
	case <-c.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("helper session was not closed")
	}
}

// setTimeout sets the global Timeout, other tests of the package leave it very short
func setTimeout(t *testing.T) {
	old := Timeout
	Timeout = 5 * time.Second
	t.Cleanup(func() { Timeout = old })
}

func TestRequestKeyframe(t *testing.T) {
	setTimeout(t)
	camera := newKeyframeCamera(t)

	conn := NewClient(camera.uri)

	// concurrent requests start only one helper session
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn.RequestKeyframe()
		}()
	}
	wg.Wait()

	camera.waitClosed(t)
	// the helper doesn't PLAY, and ends with TEARDOWN
	require.Equal(t, []string{MethodDescribe, MethodSetup, MethodTeardown}, camera.sessions())

	// a request right after the session is throttled
	require.Eventually(t, func() bool { return conn.keyframeNext.Load() != math.MaxInt64 }, 5*time.Second, time.Millisecond)
	conn.RequestKeyframe()
	time.Sleep(100 * time.Millisecond)
	require.Len(t, camera.sessions(), 3)

	// a request after the interval starts the next session
	conn.keyframeNext.Store(0)
	conn.RequestKeyframe()
	camera.waitClosed(t)
	require.Len(t, camera.sessions(), 6)
}

// GetTrack sets the keyframe handler only for video and only with the option
func TestGetTrackKeyframe(t *testing.T) {
	setTimeout(t)

	for _, tc := range []struct {
		keyframe string
		kind     string
		helper   bool
	}{
		{KeyframeResetup, core.KindVideo, true},
		{KeyframeResetup, core.KindAudio, false},
		{"", core.KindVideo, false},
	} {
		camera := newKeyframeCamera(t)

		conn := NewClient(camera.uri)
		conn.Keyframe = tc.keyframe
		require.Nil(t, conn.Dial())
		require.Nil(t, conn.Describe())
		t.Cleanup(func() { _ = conn.conn.Close() })

		var track *core.Receiver
		for _, media := range conn.Medias {
			if media.Kind == tc.kind {
				var err error
				track, err = conn.GetTrack(media, media.Codecs[0])
				require.Nil(t, err)
			}
		}
		require.NotNil(t, track)
		main := len(camera.sessions()) // DESCRIBE, SETUP of the main session

		sender := core.NewSender(nil, track.Codec)
		sender.Bind(track)
		sender.RequestKeyframe()

		if tc.helper {
			camera.waitClosed(t)
			require.Equal(t, []string{MethodDescribe, MethodSetup, MethodTeardown}, camera.sessions()[main:])
		} else {
			time.Sleep(100 * time.Millisecond)
			require.Len(t, camera.sessions(), main, "%s %s", tc.keyframe, tc.kind)
		}
	}
}

func TestRequestKeyframeError(t *testing.T) {
	setTimeout(t)
	ln, err := net.Listen("tcp", "localhost:0")
	require.Nil(t, err)
	uri := "rtsp://" + ln.Addr().String() + "/stream"
	require.Nil(t, ln.Close()) // connection refused

	conn := NewClient(uri)

	msg := make(chan any, 1)
	conn.Listen(func(m any) { msg <- m })

	conn.RequestKeyframe()

	select {
	case m := <-msg:
		require.Contains(t, m, "RTSP keyframe")
	case <-time.After(5 * time.Second):
		t.Fatal("no error")
	}
}

// a camera that never answers can't block the helper session, every request
// has the usual timeout
func TestRequestKeyframeTimeout(t *testing.T) {
	old := Timeout
	Timeout = 100 * time.Millisecond
	t.Cleanup(func() { Timeout = old })

	ln, err := net.Listen("tcp", "localhost:0")
	require.Nil(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	conn := NewClient("rtsp://" + ln.Addr().String() + "/stream")
	conn.RequestKeyframe()

	require.Eventually(t, func() bool { return conn.keyframeNext.Load() != math.MaxInt64 }, 2*time.Second, 5*time.Millisecond)
}

// the main session followed a redirect, the helper uses the final location
func TestRequestKeyframeAfterRedirect(t *testing.T) {
	setTimeout(t)

	target := newKeyframeCamera(t)
	camera := newKeyframeCamera(t)
	camera.redirect = target.uri

	conn := NewClient(camera.uri)
	require.Nil(t, conn.Dial())
	require.Nil(t, conn.Describe()) // redirected to target
	t.Cleanup(func() { _ = conn.conn.Close() })

	conn.RequestKeyframe()

	target.waitClosed(t)
	require.Equal(t, []string{MethodDescribe, MethodDescribe, MethodSetup, MethodTeardown}, target.sessions())
	require.Equal(t, []string{MethodDescribe}, camera.sessions())
}
