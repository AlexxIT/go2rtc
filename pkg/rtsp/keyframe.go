package rtsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// KeyframeResetup enables keyframe requests through a short helper session:
// OPTIONS, DESCRIBE, SETUP of the first video track, TEARDOWN (no PLAY).
// Some cameras (UniFi Protect) ignore RTCP PLI/FIR but emit an IDR frame for
// all sessions when a new session sets up the video track.
const KeyframeResetup = "resetup"

var (
	ErrKeyframeDisabled   = errors.New("rtsp: keyframe request disabled")
	ErrKeyframeNotPlaying = errors.New("rtsp: keyframe request: source not playing")
	ErrKeyframeThrottled  = errors.New("rtsp: keyframe request throttled")
	ErrKeyframeBusy       = errors.New("rtsp: keyframe request: connection busy")
	ErrKeyframeClosed     = errors.New("rtsp: keyframe request: connection closed")
	// ErrKeyframeTransport is returned for sources with a WebSocket transport:
	// the dial of that transport has no timeout, a helper session could hang
	// in it for good.
	ErrKeyframeTransport = errors.New("rtsp: keyframe request: transport not supported")
	// ErrKeyframeFailed is the root of every error of the helper session.
	// The message carries only the stage and an error class, never the
	// text of the inner error (it may contain the URL, credentials or host).
	ErrKeyframeFailed = errors.New("rtsp: keyframe helper failed")
)

var (
	// keyframeInterval is the minimum time between two helper sessions.
	keyframeInterval = time.Second
	// keyframeTimeout limits the RTSP exchange of one helper session after dial.
	keyframeTimeout = 2 * time.Second
)

var errKeyframeNoVideo = errors.New("rtsp: no video media")

var _ core.KeyframeRequester = (*Conn)(nil)

// RequestKeyframe starts a helper session that makes the source emit a
// keyframe. It never blocks: the session runs in its own goroutine, the
// result is reported through OnKeyframe. At most one helper session runs at a
// time and at most one starts per keyframeInterval; other requests are dropped.
// Both limits apply per connection: after a reconnect of the source the new
// connection starts with fresh limits. Stop of the old connection aborts its
// helper session in every phase (TCP connect, TLS handshake, RTSP exchange)
// but does not wait for it, so the two overlap at most for the moment the
// aborted helper needs to return.
func (c *Conn) RequestKeyframe() error {
	if c.Keyframe != KeyframeResetup {
		return ErrKeyframeDisabled
	}

	if c.Transport != "" && c.Transport != "tcp" && c.Transport != "udp" {
		return ErrKeyframeTransport
	}

	// the caller is the RTCP read loop: never wait for a lock.
	// Lock order is kfMu before stateMu; nobody takes kfMu under stateMu.
	// kfMu is only held briefly, so losing the TryLock race is rare; the
	// request is dropped like a throttled one and the viewer asks again.
	if !c.kfMu.TryLock() {
		return ErrKeyframeBusy
	}
	defer c.kfMu.Unlock()

	if c.kfClosed {
		return ErrKeyframeClosed
	}

	// stateMu is held over network I/O (Reconnect, Play, TEARDOWN)
	if !c.stateMu.TryLock() {
		return ErrKeyframeBusy
	}
	if c.mode != core.ModeActiveProducer || c.state != StatePlay {
		c.stateMu.Unlock()
		return ErrKeyframeNotPlaying
	}
	uri := c.uri
	c.stateMu.Unlock()

	now := c.keyframeNow()

	if c.kfBusy || now.Sub(c.kfLast) < keyframeInterval {
		return ErrKeyframeThrottled
	}

	ctx, cancel := context.WithCancel(context.Background())

	c.kfLast = now
	c.kfBusy = true
	c.kfCancel = cancel
	c.kfWG.Add(1)
	go c.runKeyframeHelper(ctx, uri)

	return nil
}

func (c *Conn) keyframeNow() time.Time {
	if c.kfNow != nil {
		return c.kfNow()
	}
	return time.Now()
}

func (c *Conn) runKeyframeHelper(ctx context.Context, uri string) {
	start := time.Now()

	defer c.kfWG.Done()

	err := c.keyframeSession(ctx, uri)

	c.kfMu.Lock()
	cancel := c.kfCancel
	c.kfRaw = nil
	c.kfCancel = nil
	c.kfBusy = false
	c.kfMu.Unlock()

	cancel() // releases the context; stopKeyframe may have called it already

	if c.OnKeyframe != nil {
		c.OnKeyframe(err, time.Since(start))
	}
}

func (c *Conn) keyframeSession(ctx context.Context, uri string) (err error) {
	helper := NewClient(uri)
	helper.UserAgent = c.UserAgent
	helper.Timeout = c.Timeout
	// Never UDP: the helper never plays, so a UDP port pair would be opened
	// for nothing. RequestKeyframe has already refused WebSocket transports.
	if c.Transport != "udp" {
		helper.Transport = c.Transport
	}
	// Backchannel must stay false: SetupMedia redials when SETUP fails on a
	// backchannel connection, and that new connection would be neither in kfRaw
	// nor under the deadline below.
	helper.Backchannel = false
	helper.noRedirect = true

	// ctx is cancelled by stopKeyframe: Stop ends a helper that is still in
	// TCP connect or TLS handshake at once, not only after the dial timeout
	if err = helper.dial(ctx); err != nil {
		return keyframeError("dial", err, false)
	}

	raw := helper.conn

	c.kfMu.Lock()
	if c.kfClosed {
		c.kfMu.Unlock()
		_ = raw.Close()
		return fmt.Errorf("%w: %w", ErrKeyframeFailed, ErrKeyframeClosed)
	}
	c.kfRaw = raw
	c.kfMu.Unlock()

	// the helper session never dials again (no redirect, no reconnect),
	// so closing this raw connection ends every blocking call of it
	var timedOut atomic.Bool
	timer := time.AfterFunc(keyframeTimeout, func() {
		timedOut.Store(true)
		_ = raw.Close()
	})
	defer timer.Stop()

	defer func() {
		_ = helper.Close() // sends TEARDOWN once Describe has set the mode
	}()

	if err = helper.Options(); err != nil {
		return keyframeError("options", err, timedOut.Load())
	}
	if err = helper.Describe(); err != nil {
		return keyframeError("describe", err, timedOut.Load())
	}

	var video *core.Media
	for _, media := range helper.Medias {
		if media.Kind == core.KindVideo && media.Direction == core.DirectionRecvonly {
			video = media
			break
		}
	}
	if video == nil {
		return keyframeError("setup", errKeyframeNoVideo, false)
	}

	if _, err = helper.SetupMedia(video); err != nil {
		return keyframeError("setup", err, timedOut.Load())
	}

	return nil
}

// stopKeyframe marks the connection closed, aborts the dial of a running
// helper session and closes its raw connection. It does not wait for the
// helper goroutine; that one ends right after (no blocking call remains).
func (c *Conn) stopKeyframe() {
	c.kfMu.Lock()
	c.kfClosed = true
	raw := c.kfRaw
	cancel := c.kfCancel
	c.kfMu.Unlock()

	if cancel != nil {
		cancel()
	}

	// close outside kfMu: a *tls.Conn sends close_notify with a write deadline,
	// RequestKeyframe must not wait for that
	if raw != nil {
		_ = raw.Close() // net.Conn is safe for concurrent use
	}
}

// keyframeError builds an error from stage and error class only.
// The inner error is deliberately not wrapped: its text may contain
// the source URL, credentials, the URL path or host:port.
func keyframeError(stage string, err error, timedOut bool) error {
	class := keyframeErrorClass(err)
	if timedOut {
		class = "timeout"
	}
	return fmt.Errorf("%w: %s: %s", ErrKeyframeFailed, stage, class)
}

func keyframeErrorClass(err error) string {
	var netErr net.Error
	var urlErr *url.Error
	var resErr *responseError

	switch {
	case errors.Is(err, os.ErrDeadlineExceeded),
		errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	case errors.Is(err, net.ErrClosed), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF),
		errors.Is(err, context.Canceled): // dial aborted by stopKeyframe
		return "closed"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "refused"
	case errors.Is(err, errAuthMissing), errors.Is(err, errAuthWrong),
		errors.As(err, &resErr) && resErr.code == 401:
		return "auth"
	case errors.Is(err, errRedirectRefused):
		return "redirect"
	case errors.As(err, &urlErr):
		return "parse"
	case errors.As(err, &resErr):
		return "response"
	case errors.Is(err, errKeyframeNoVideo):
		return "no-video"
	}
	return "other"
}
