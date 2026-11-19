package rtsp

import (
	"math"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// Some cameras (ex. UniFi Protect) ignore RTCP PLI/FIR, but send a keyframe to
// every session when a new session sets up the video track.

const (
	KeyframeResetup = "resetup"

	keyframeInterval = time.Second
)

// RequestKeyframe asks the camera for a keyframe with a short helper session.
// GetTrack sets it as the keyframe handler of video tracks with the option
// `#keyframe=resetup`. It never blocks and never touches the main session.
// Helper sessions don't overlap, and start at least keyframeInterval apart.
func (c *Conn) RequestKeyframe() {
	now := time.Now().UnixNano()
	next := c.keyframeNext.Load()
	if now < next || !c.keyframeNext.CompareAndSwap(next, math.MaxInt64) {
		return
	}

	go func() {
		err := c.keyframeSession()
		c.keyframeNext.Store(time.Now().Add(keyframeInterval).UnixNano())
		if err != nil {
			c.Fire("RTSP keyframe: " + err.Error())
		}
	}()
}

// keyframeSession: DESCRIBE, SETUP video, TEARDOWN. There is no PLAY, the
// camera sends the keyframe on SETUP. Only a TCP session is used, every
// request has the usual timeout.
func (c *Conn) keyframeSession() error {
	switch c.Transport {
	case "", "tcp", "udp":
	default:
		return nil // ex. RTSP over WebSocket has no dial timeout
	}

	// c.uri changes on a redirect of the main session, after the tracks exist
	// only under stateMu (GetTrack, Start)
	c.stateMu.Lock()
	uri := c.uri
	c.stateMu.Unlock()

	h := NewClient(uri)
	h.UserAgent = c.UserAgent
	h.Timeout = c.Timeout

	if err := h.Dial(); err != nil {
		return err
	}
	defer h.Close() // sends TEARDOWN after DESCRIBE, then closes the connection

	if err := h.Describe(); err != nil {
		return err
	}

	for _, media := range h.Medias {
		if media.Kind == core.KindVideo && media.Direction == core.DirectionRecvonly {
			_, err := h.SetupMedia(media)
			return err
		}
	}

	return nil
}
