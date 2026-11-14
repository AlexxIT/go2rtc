package rtsp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

// fakeRTSPServer answers just enough RTSP for one main session plus helper
// sessions. Its behaviour can be changed after the main session is playing.
type fakeRTSPServer struct {
	t  *testing.T
	ln net.Listener

	mu             sync.Mutex
	describeStatus int
	setupStatus    int
	contentBase    string
	hangSetup      bool
	redirectSetup  string
	counts         map[string]int
	open           map[net.Conn]struct{}
	active         int
	maxActive      int // max simultaneous connections minus the first one
	accepted       int
	setupTransport []string // Transport header of every SETUP request

	wg sync.WaitGroup
}

func newFakeRTSPServer(t *testing.T) *fakeRTSPServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := &fakeRTSPServer{
		t:              t,
		ln:             ln,
		describeStatus: 200,
		setupStatus:    200,
		counts:         map[string]int{},
		open:           map[net.Conn]struct{}{},
	}

	s.wg.Add(1)
	go s.serve()

	t.Cleanup(func() {
		_ = ln.Close()
		s.mu.Lock()
		for conn := range s.open {
			_ = conn.Close()
		}
		s.mu.Unlock()
		s.wg.Wait()
	})

	return s
}

func (s *fakeRTSPServer) addr() string { return s.ln.Addr().String() }

func (s *fakeRTSPServer) set(f func(s *fakeRTSPServer)) {
	s.mu.Lock()
	f(s)
	s.mu.Unlock()
}

func (s *fakeRTSPServer) count(method string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[method]
}

func (s *fakeRTSPServer) conns() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (s *fakeRTSPServer) maxHelpers() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxActive
}

func (s *fakeRTSPServer) acceptedConns() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accepted
}

func (s *fakeRTSPServer) setupTransports() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.setupTransport...)
}

func (s *fakeRTSPServer) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}

		s.mu.Lock()
		s.open[conn] = struct{}{}
		s.accepted++
		s.active++
		if s.active-1 > s.maxActive {
			s.maxActive = s.active - 1
		}
		s.mu.Unlock()

		s.wg.Add(1)
		go s.handle(conn)
	}
}

func (s *fakeRTSPServer) handle(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		_ = conn.Close()
		s.mu.Lock()
		delete(s.open, conn)
		s.active--
		s.mu.Unlock()
	}()

	r := bufio.NewReader(conn)
	for {
		// read one request up to the empty line
		var method, cseq, transport string
		for first := true; ; first = false {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			if first {
				method, _, _ = strings.Cut(line, " ")
			} else if k, v, ok := strings.Cut(line, ":"); ok {
				switch {
				case strings.EqualFold(k, "CSeq"):
					cseq = strings.TrimSpace(v)
				case strings.EqualFold(k, "Transport"):
					transport = strings.TrimSpace(v)
				}
			}
		}

		s.mu.Lock()
		s.counts[method]++
		if method == MethodSetup {
			s.setupTransport = append(s.setupTransport, transport)
		}
		describeStatus := s.describeStatus
		setupStatus := s.setupStatus
		contentBase := s.contentBase
		hangSetup := s.hangSetup
		redirectSetup := s.redirectSetup
		s.mu.Unlock()

		var res string
		switch method {
		case MethodOptions, MethodPlay:
			res = "RTSP/1.0 200 OK\r\nCSeq: " + cseq + "\r\n\r\n"
		case MethodDescribe:
			if describeStatus != 200 {
				res = fmt.Sprintf("RTSP/1.0 %d Error\r\nCSeq: %s\r\n\r\n", describeStatus, cseq)
				break
			}
			sdp := "v=0\r\no=- 1 1 IN IP4 0.0.0.0\r\ns=fake\r\nt=0 0\r\n" +
				"m=video 0 RTP/AVP 96\r\na=rtpmap:96 H264/90000\r\na=control:trackID=0\r\n"
			res = "RTSP/1.0 200 OK\r\nCSeq: " + cseq + "\r\nContent-Type: application/sdp\r\n"
			if contentBase != "" {
				res += "Content-Base: " + contentBase + "\r\n"
			}
			res += fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(sdp), sdp)
		case MethodSetup:
			if hangSetup {
				continue
			}
			if redirectSetup != "" {
				res = "RTSP/1.0 302 Found\r\nCSeq: " + cseq + "\r\nLocation: " + redirectSetup + "\r\n\r\n"
				break
			}
			if setupStatus != 200 {
				res = fmt.Sprintf("RTSP/1.0 %d Error\r\nCSeq: %s\r\n\r\n", setupStatus, cseq)
				break
			}
			res = "RTSP/1.0 200 OK\r\nCSeq: " + cseq +
				"\r\nSession: 1\r\nTransport: RTP/AVP/TCP;unicast;interleaved=0-1\r\n\r\n"
		case MethodTeardown:
			continue
		default:
			res = "RTSP/1.0 501 Not Implemented\r\nCSeq: " + cseq + "\r\n\r\n"
		}

		if _, err := conn.Write([]byte(res)); err != nil {
			return
		}
	}
}

// withKeyframeGlobals saves and restores the package wide timing values.
// Register it before newPlayingConn so that its cleanup runs last.
func withKeyframeGlobals(t *testing.T, interval, timeout time.Duration) {
	t.Helper()
	oldTimeout, oldInterval, oldKfTimeout := Timeout, keyframeInterval, keyframeTimeout
	Timeout = 2 * time.Second
	keyframeInterval = interval
	keyframeTimeout = timeout
	t.Cleanup(func() {
		Timeout, keyframeInterval, keyframeTimeout = oldTimeout, oldInterval, oldKfTimeout
	})
}

// fakeClock is a settable clock for the throttle.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(c *Conn) *fakeClock {
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	c.kfNow = clock.Now
	return clock
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Add(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	f.mu.Unlock()
}

// keyframeResults collects OnKeyframe callbacks.
type keyframeResults struct {
	ch chan error
	n  atomic.Int32
}

func watchKeyframe(c *Conn) *keyframeResults {
	r := &keyframeResults{ch: make(chan error, 1000)}
	c.OnKeyframe = func(err error, took time.Duration) {
		r.n.Add(1)
		r.ch <- err
	}
	return r
}

func (r *keyframeResults) wait(t *testing.T, d time.Duration) error {
	t.Helper()
	select {
	case err := <-r.ch:
		return err
	case <-time.After(d):
		t.Fatalf("no OnKeyframe callback within %v", d)
		return nil
	}
}

func testURL(srv *fakeRTSPServer) string {
	return "rtsp://user:pass@" + srv.addr() + "/SECRETPATHTOKEN"
}

// newSetupConn returns a connection after DESCRIBE and SETUP, before PLAY.
func newSetupConn(t *testing.T, srv *fakeRTSPServer) *Conn {
	t.Helper()

	c := NewClient(testURL(srv))
	c.Keyframe = KeyframeResetup
	require.NoError(t, c.Dial())
	require.NoError(t, c.Describe())

	var video *core.Media
	for _, media := range c.Medias {
		if media.Kind == core.KindVideo {
			video = media
		}
	}
	require.NotNil(t, video)
	_, err := c.GetTrack(video, video.Codecs[0])
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = c.Stop()
		waitKeyframeHelpers(t, c)
	})
	return c
}

// newPlayingConn returns a connection in state PLAY with its read loop running.
func newPlayingConn(t *testing.T, srv *fakeRTSPServer) *Conn {
	t.Helper()
	c := newSetupConn(t, srv)
	startPlaying(t, srv, c)
	return c
}

// startPlaying runs Start of a connection from newSetupConn and waits for PLAY.
func startPlaying(t *testing.T, srv *fakeRTSPServer, c *Conn) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		_ = c.Start()
		close(done)
	}()
	playing.Store(c, done)
	t.Cleanup(func() { endPlaying(t, c) })

	require.Eventually(t, func() bool {
		c.stateMu.Lock()
		defer c.stateMu.Unlock()
		return c.state == StatePlay
	}, 2*time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool { return srv.count(MethodPlay) == 1 }, 2*time.Second, 5*time.Millisecond)
}

var playing sync.Map // *Conn -> chan struct{} closed when Start returned

// endPlaying ends the read loop of a playing connection before Stop is called.
// Handle reads c.state without stateMu (upstream code), so a Stop while Handle
// runs is a data race under -race that has nothing to do with this feature.
// Closing the raw connection makes Handle and Start return first.
func endPlaying(t *testing.T, c *Conn) {
	t.Helper()
	v, ok := playing.LoadAndDelete(c)
	if !ok {
		return
	}
	_ = c.conn.Close()
	select {
	case <-v.(chan struct{}):
	case <-time.After(3 * time.Second):
		t.Error("Start did not return after the connection was closed")
	}
}

func waitKeyframeHelpers(t *testing.T, c *Conn) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		c.kfWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("keyframe helper goroutine did not end")
	}
}

func assertNoLeak(t *testing.T, srv *fakeRTSPServer, err error) {
	t.Helper()
	require.Error(t, err)
	msg := err.Error()
	require.NotContains(t, msg, "user:pass")
	require.NotContains(t, msg, "SECRETPATHTOKEN")
	require.NotContains(t, msg, srv.addr())
	require.NotContains(t, msg, "127.0.0.1")
	require.ErrorIs(t, err, ErrKeyframeFailed)
}

func TestRequestKeyframeOpensHelperSession(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)
	res := watchKeyframe(c)

	require.NoError(t, c.RequestKeyframe())

	require.NoError(t, res.wait(t, 2*time.Second))
	require.Eventually(t, func() bool { return srv.conns() == 1 }, 2*time.Second, 5*time.Millisecond)
	require.Equal(t, 1, srv.count(MethodOptions))
	require.Equal(t, 2, srv.count(MethodDescribe))
	require.Equal(t, 2, srv.count(MethodSetup))
	require.Equal(t, 1, srv.count(MethodPlay))
	require.Equal(t, 1, srv.count(MethodTeardown))
	require.Equal(t, int32(1), res.n.Load())
}

func TestRequestKeyframeDisabledByDefault(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)
	c.Keyframe = ""

	require.ErrorIs(t, c.RequestKeyframe(), ErrKeyframeDisabled)
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, 1, srv.count(MethodDescribe))
}

func TestRequestKeyframeNotPlaying(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newSetupConn(t, srv)

	require.ErrorIs(t, c.RequestKeyframe(), ErrKeyframeNotPlaying)
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, 1, srv.acceptedConns())
}

func TestRequestKeyframeDoesNotBlockOnStateLock(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)

	c.stateMu.Lock()
	result := make(chan error, 1)
	go func() { result <- c.RequestKeyframe() }()

	select {
	case err := <-result:
		c.stateMu.Unlock()
		require.ErrorIs(t, err, ErrKeyframeBusy)
	case <-time.After(50 * time.Millisecond):
		c.stateMu.Unlock()
		<-result
		t.Fatal("RequestKeyframe blocked on the state lock")
	}
}

func TestRequestKeyframeThrottled(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)
	clock := newFakeClock(c)
	res := watchKeyframe(c)

	require.NoError(t, c.RequestKeyframe())
	// wait for the end of the helper session, so that only the interval
	// (not the running session) can reject the next request
	require.NoError(t, res.wait(t, 2*time.Second))
	require.Eventually(t, func() bool { return srv.conns() == 1 }, 2*time.Second, 5*time.Millisecond)

	clock.Add(500 * time.Millisecond)
	require.ErrorIs(t, c.RequestKeyframe(), ErrKeyframeThrottled)

	clock.Add(501 * time.Millisecond)
	require.NoError(t, c.RequestKeyframe())
	require.NoError(t, res.wait(t, 2*time.Second))

	require.Equal(t, 3, srv.count(MethodSetup))
}

func TestRequestKeyframeNeverParallel(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)
	clock := newFakeClock(c)
	srv.set(func(s *fakeRTSPServer) { s.hangSetup = true })

	require.NoError(t, c.RequestKeyframe())
	require.Eventually(t, func() bool { return srv.count(MethodSetup) == 2 }, 2*time.Second, 5*time.Millisecond)

	clock.Add(5 * time.Second)
	require.ErrorIs(t, c.RequestKeyframe(), ErrKeyframeThrottled)
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, 1, srv.maxHelpers())
}

// floodKeyframe calls RequestKeyframe from 50 goroutines for d.
func floodKeyframe(c *Conn, d time.Duration) {
	var wg sync.WaitGroup
	stop := time.Now().Add(d)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(stop) {
				_ = c.RequestKeyframe()
			}
		}()
	}
	wg.Wait()
}

// The bounds are checked at points the test controls, not by timing: the
// server counts connections when it accepts them, and the throttle clock only
// moves when the test moves it. (Counting open connections on the server lags
// behind the client closing them and made this test flaky under CPU load.)
func TestRequestKeyframeFloodIsBounded(t *testing.T) {
	t.Run("one helper at a time", func(t *testing.T) {
		withKeyframeGlobals(t, time.Second, 10*time.Second)
		srv := newFakeRTSPServer(t)
		c := newPlayingConn(t, srv)
		clock := newFakeClock(c)
		srv.set(func(s *fakeRTSPServer) { s.hangSetup = true })

		// this helper hangs in SETUP and runs for the whole flood
		require.NoError(t, c.RequestKeyframe())
		require.Eventually(t, func() bool { return srv.count(MethodSetup) == 2 }, 2*time.Second, 5*time.Millisecond)

		clock.Add(time.Hour) // only the running helper can reject a request now
		floodKeyframe(c, 500*time.Millisecond)

		require.Equal(t, 2, srv.acceptedConns(), "main session plus one helper")
		require.Equal(t, 2, srv.count(MethodSetup))
	})

	t.Run("one helper per interval", func(t *testing.T) {
		withKeyframeGlobals(t, time.Second, 2*time.Second)
		srv := newFakeRTSPServer(t)
		c := newPlayingConn(t, srv)
		clock := newFakeClock(c) // frozen: the interval never passes during the flood
		res := watchKeyframe(c)

		// the first helper has ended before the flood (OnKeyframe runs after
		// kfBusy is cleared), so only the interval can reject a request
		require.NoError(t, c.RequestKeyframe())
		require.NoError(t, res.wait(t, 2*time.Second))
		floodKeyframe(c, 500*time.Millisecond)

		require.Equal(t, int32(1), res.n.Load())
		require.Equal(t, 2, srv.acceptedConns(), "main session plus one helper")
		require.Equal(t, 2, srv.count(MethodSetup))

		// and it is the interval: once it has passed, the next request starts a helper
		clock.Add(time.Second)
		require.NoError(t, c.RequestKeyframe())
		require.NoError(t, res.wait(t, 2*time.Second))
	})
}

func TestKeyframeHelperErrorReported(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)
	res := watchKeyframe(c)
	srv.set(func(s *fakeRTSPServer) { s.setupStatus = 500 })

	require.NoError(t, c.RequestKeyframe())
	err := res.wait(t, 2*time.Second)
	assertNoLeak(t, srv, err)
	require.Contains(t, err.Error(), "setup: response")

	require.Eventually(t, func() bool { return srv.conns() == 1 }, 2*time.Second, 5*time.Millisecond)
	c.stateMu.Lock()
	state := c.state
	c.stateMu.Unlock()
	require.Equal(t, StatePlay, state)
}

// The helper session must never redial. SetupMedia redials once when SETUP
// fails on a connection with Backchannel set; that new connection would be
// neither in kfRaw nor under the helper deadline.
func TestKeyframeHelperNoRedialOnSetupError(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)
	c.Backchannel = true // normal for RTSP sources; the helper must not inherit it
	res := watchKeyframe(c)
	srv.set(func(s *fakeRTSPServer) { s.setupStatus = 500 })

	require.NoError(t, c.RequestKeyframe())
	err := res.wait(t, 2*time.Second)
	assertNoLeak(t, srv, err)
	require.Contains(t, err.Error(), "setup: response")

	// the main connection plus exactly one helper connection
	require.Equal(t, 2, srv.acceptedConns())
	require.Equal(t, 2, srv.count(MethodSetup))
}

// The helper session uses TCP-interleaved even if the source uses UDP:
// it never plays, so a UDP port pair would be opened for nothing.
func TestKeyframeHelperNeverUDP(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newSetupConn(t, srv)
	c.Transport = "udp" // set after the main SETUP and before Start
	startPlaying(t, srv, c)
	res := watchKeyframe(c)

	require.NoError(t, c.RequestKeyframe())
	require.NoError(t, res.wait(t, 2*time.Second))

	transports := srv.setupTransports()
	require.Len(t, transports, 2)
	require.Contains(t, transports[1], "RTP/AVP/TCP")
	require.NotContains(t, transports[1], "client_port")
}

// A WebSocket transport has no dial timeout, so a helper session could hang
// in the dial for good: the request is refused before anything is dialed.
func TestRequestKeyframeRefusedForWebSocketTransport(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newSetupConn(t, srv)
	c.Transport = "ws://127.0.0.1:1/never-dialed" // set after the main SETUP and before Start
	startPlaying(t, srv, c)

	require.ErrorIs(t, c.RequestKeyframe(), ErrKeyframeTransport)
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, 1, srv.acceptedConns())
}

func TestKeyframeHelperUnauthorized(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)
	res := watchKeyframe(c)
	srv.set(func(s *fakeRTSPServer) { s.describeStatus = 401 })

	require.NoError(t, c.RequestKeyframe())
	err := res.wait(t, 2*time.Second)
	assertNoLeak(t, srv, err)
	require.Contains(t, err.Error(), "describe: auth")
}

func TestKeyframeHelperBadContentBase(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)
	res := watchKeyframe(c)
	srv.set(func(s *fakeRTSPServer) { s.contentBase = "rtsp://user:pass@h/SECRETPATHTOKEN/%zz" })

	require.NoError(t, c.RequestKeyframe())
	err := res.wait(t, 2*time.Second)
	assertNoLeak(t, srv, err)
	require.Contains(t, err.Error(), "describe: parse")
}

func TestKeyframeHelperRedirectRejected(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	other := newFakeRTSPServer(t)
	other.set(func(s *fakeRTSPServer) { s.hangSetup = true })
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)
	res := watchKeyframe(c)
	srv.set(func(s *fakeRTSPServer) {
		s.redirectSetup = "rtsp://" + other.addr() + "/SECRETPATHTOKEN/trackID=0"
	})

	require.NoError(t, c.RequestKeyframe())
	err := res.wait(t, time.Second)
	assertNoLeak(t, srv, err)
	require.NotContains(t, err.Error(), other.addr())
	require.Contains(t, err.Error(), "setup: redirect")
	require.Equal(t, 0, other.acceptedConns())
}

func TestKeyframeHelperTimeout(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 200*time.Millisecond)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)
	res := watchKeyframe(c)
	srv.set(func(s *fakeRTSPServer) { s.hangSetup = true })

	require.NoError(t, c.RequestKeyframe())
	err := res.wait(t, time.Second)
	assertNoLeak(t, srv, err)
	require.Contains(t, err.Error(), "setup: timeout")
	require.Eventually(t, func() bool { return srv.conns() == 1 }, time.Second, 5*time.Millisecond)
}

func TestKeyframeHelperNetworkError(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)
	clock := newFakeClock(c)
	res := watchKeyframe(c)
	require.NoError(t, srv.ln.Close())

	require.NoError(t, c.RequestKeyframe())
	err := res.wait(t, 2*time.Second)
	assertNoLeak(t, srv, err)
	require.Contains(t, err.Error(), "dial: refused")

	clock.Add(time.Second)
	require.NoError(t, c.RequestKeyframe())
	assertNoLeak(t, srv, res.wait(t, 2*time.Second))
}

func TestStopCancelsKeyframeHelper(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)
	clock := newFakeClock(c)
	res := watchKeyframe(c)
	srv.set(func(s *fakeRTSPServer) { s.hangSetup = true })

	require.NoError(t, c.RequestKeyframe())
	require.Eventually(t, func() bool { return srv.count(MethodSetup) == 2 }, 2*time.Second, 5*time.Millisecond)

	endPlaying(t, c)
	_ = c.Stop() // reports the close error of the already closed main connection

	require.Eventually(t, func() bool { return srv.conns() == 0 }, 500*time.Millisecond, 5*time.Millisecond)
	// Stop only closes the raw socket of the helper session. Running the
	// helper's RTSP Close from Stop would write a TEARDOWN from a second
	// goroutine; -race does not see that (net.Conn writes synchronize), the
	// request count does. The main connection was closed before Stop.
	require.Equal(t, 0, srv.count(MethodTeardown))
	err := res.wait(t, 500*time.Millisecond)
	require.ErrorIs(t, err, ErrKeyframeFailed)
	assertNoLeak(t, srv, err)

	done := make(chan struct{})
	go func() { c.kfWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("keyframe helper goroutine still running after Stop")
	}

	accepted := srv.acceptedConns()
	clock.Add(5 * time.Second)
	require.ErrorIs(t, c.RequestKeyframe(), ErrKeyframeClosed)
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, accepted, srv.acceptedConns())
}

func TestRequestKeyframeDoesNotBlockOnKeyframeLock(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)

	c.kfMu.Lock()
	result := make(chan error, 1)
	go func() { result <- c.RequestKeyframe() }()

	select {
	case err := <-result:
		c.kfMu.Unlock()
		require.ErrorIs(t, err, ErrKeyframeBusy)
	case <-time.After(50 * time.Millisecond):
		c.kfMu.Unlock()
		<-result
		t.Fatal("RequestKeyframe blocked on the keyframe lock")
	}
}

// blockingConn is a net.Conn whose first Close blocks until released,
// like a *tls.Conn sending close_notify to a peer that does not read.
type blockingConn struct {
	net.Conn
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingConn) Close() error {
	b.once.Do(func() {
		close(b.entered)
		<-b.release
	})
	return nil
}

func TestStopKeyframeClosesOutsideLock(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newPlayingConn(t, srv)

	raw := &blockingConn{entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() { close(raw.release) })

	c.kfMu.Lock()
	c.kfRaw = raw
	c.kfMu.Unlock()

	go c.stopKeyframe()
	select {
	case <-raw.entered:
	case <-time.After(time.Second):
		t.Fatal("stopKeyframe did not close the helper connection")
	}

	// Close is still blocked: a request must neither wait nor see the lock taken
	result := make(chan error, 1)
	go func() { result <- c.RequestKeyframe() }()
	select {
	case err := <-result:
		require.ErrorIs(t, err, ErrKeyframeClosed)
	case <-time.After(50 * time.Millisecond):
		t.Fatal("RequestKeyframe blocked while the helper connection was closing")
	}
}

// stopDeadline is how fast Stop must end a helper session that is still in
// its dial phase. The dial timeout in these tests is several seconds, so a
// helper that only ends by its timeout fails clearly.
const stopDeadline = 200 * time.Millisecond

// assertStopEndsDialingHelper points the helper session at uri, waits until
// it is in its dial phase, stops the connection and checks that the helper
// ends within stopDeadline instead of the dial timeout.
func assertStopEndsDialingHelper(t *testing.T, srv *fakeRTSPServer, c *Conn, uri string, dialing <-chan struct{}) {
	t.Helper()
	res := watchKeyframe(c)

	c.stateMu.Lock()
	c.uri = uri
	c.stateMu.Unlock()

	require.NoError(t, c.RequestKeyframe())
	select {
	case <-dialing:
	case <-time.After(time.Second):
		t.Fatal("helper did not reach its dial phase")
	}

	endPlaying(t, c)
	stopped := time.Now()
	_ = c.Stop()

	var err error
	select {
	case err = <-res.ch:
	case <-time.After(stopDeadline):
		t.Fatalf("helper in dial phase still running %v after Stop", stopDeadline)
	}
	t.Logf("helper ended %v after Stop", time.Since(stopped))
	assertNoLeak(t, srv, err)
	require.Contains(t, err.Error(), "dial: closed")
	waitKeyframeHelpers(t, c)

	c.kfMu.Lock()
	busy, cancel := c.kfBusy, c.kfCancel
	c.kfMu.Unlock()
	require.False(t, busy)
	require.Nil(t, cancel)
	require.ErrorIs(t, c.RequestKeyframe(), ErrKeyframeClosed)
}

func TestStopEndsKeyframeHelperInTLSHandshake(t *testing.T) {
	withKeyframeGlobals(t, time.Second, 2*time.Second)
	srv := newFakeRTSPServer(t)
	c := newSetupConn(t, srv)
	c.Timeout = 5 // dial and handshake timeout of the helper in seconds; set before Start
	startPlaying(t, srv, c)

	// a peer that accepts TCP but never answers the TLS handshake
	silent, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	accepted := make(chan net.Conn, 4)
	dialing := make(chan struct{})
	go func() {
		for {
			conn, err := silent.Accept()
			if err != nil {
				return
			}
			accepted <- conn
			close(dialing)
		}
	}()
	t.Cleanup(func() { _ = silent.Close() })

	assertStopEndsDialingHelper(t, srv, c,
		"rtspx://user:pass@"+silent.Addr().String()+"/SECRETPATHTOKEN", dialing)

	// the helper must have closed its socket, not just returned
	conn := <-accepted
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(stopDeadline))
	_, err = conn.Read(make([]byte, 512)) // ClientHello first, then EOF
	for err == nil {
		_, err = conn.Read(make([]byte, 512))
	}
	require.ErrorIs(t, err, io.EOF, "helper socket still open after Stop")
}
