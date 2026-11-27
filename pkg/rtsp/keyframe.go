package rtsp

import (
	"math"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtcp"
)

// Some cameras (ex. UniFi Protect) ignore RTCP PLI/FIR, but send a keyframe to
// every session when a new session sets up the video track.

const (
	KeyframeReSETUP = "resetup"
	KeyframeRePLAY  = "replay"
	KeyframePLAY    = "play"
	KeyframePLI     = "pli" // RTCP Picture Loss Indication (RFC 4585)
	KeyframeFIR     = "fir" // RTCP Full Intra Request (RFC 5104)

	keyframeInterval = time.Second
)

func (c *Conn) RequestKeyframe() {
	now := time.Now().UnixNano()
	next := c.keyframeNext.Load()
	if now < next || !c.keyframeNext.CompareAndSwap(next, math.MaxInt64) {
		return
	}

	var err error

	switch c.Keyframe {
	case KeyframePLAY:
		err = c.Play()
	case KeyframeReSETUP, KeyframeRePLAY:
		err = c.keyframeSession()
	case KeyframePLI, KeyframeFIR:
		err = c.requestKeyframeRTCP()
	}

	if err != nil {
		c.Fire("RTSP keyframe: " + err.Error())
	}

	c.keyframeNext.Store(time.Now().Add(keyframeInterval).UnixNano())
}

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
			if _, err := h.SetupMedia(media); err != nil {
				return err
			}
			break
		}
	}

	if c.Keyframe == KeyframeRePLAY {
		return h.Play()
	}

	return nil
}

// requestKeyframeRTCP asks the camera for a keyframe with RTCP PLI or FIR on
// the main session. GetTrack sets it as the keyframe handler of video tracks
// with the option `#keyframe=pli` or `#keyframe=fir`. Requests are at least
// keyframeInterval apart, and nothing is sent before the track got RTP
// (the camera's SSRC is unknown until then).
func (c *Conn) requestKeyframeRTCP() error {
	channel := byte(c.keyframeChan.Load())
	ssrc := c.keyframeSSRC.Load()
	if ssrc == 0 {
		return nil
	}

	if c.Transport == "udp" && int(channel)+1 >= len(c.udpAddr) {
		return nil // the camera gave no RTCP port
	}

	var pkt rtcp.Packet

	switch c.Keyframe {
	case KeyframePLI:
		pkt = &rtcp.PictureLossIndication{MediaSSRC: ssrc}
	case KeyframeFIR:
		seq := uint8(c.firSeq.Add(1))
		pkt = &rtcp.FullIntraRequest{FIR: []rtcp.FIREntry{{SSRC: ssrc, SequenceNumber: seq}}}
	}

	b, err := pkt.Marshal()
	if err != nil {
		return err
	}

	// RTCP goes on the odd channel after the track's RTP channel
	data := append([]byte{'$', channel + 1, byte(len(b) >> 8), byte(len(b))}, b...)
	return c.writeInterleavedData(data)
}
