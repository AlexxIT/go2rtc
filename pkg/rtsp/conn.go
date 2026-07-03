package rtsp

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

type Conn struct {
	core.Connection
	core.Listener

	// public

	Backchannel bool
	Media       string
	OnClose     func() error
	PacketSize  uint16
	SessionName string
	Timeout     int
	Transport   string // custom transport support, ex. RTSP over WebSocket

	URL *url.URL

	// internal

	auth      *tcp.Auth
	conn      net.Conn
	keepalive int
	mode      core.Mode
	playOK    bool
	reader    *bufio.Reader
	sequence  int
	session   string
	uri       string

	state   State
	stateMu sync.Mutex

	// audio->video re-clock (go2rtc#2303). On some G.711+H.265 cams the audio
	// RTP clock runs slower than the video clock, so a long-running consumer's
	// muxer interleave gap grows until audio is starved to silence. We peg the
	// audio packet timestamp to VIDEO's elapsed time so the two can't drift
	// apart. Video timestamps are never touched. Single producer read goroutine
	// updates these, so no locking needed.
	vidHave    bool
	vidLastTS  uint32
	vidTicks   int64 // accumulated video RTP ticks since first video pkt (wrap-safe)
	vidClock   float64
	vidElapsed float64 // seconds of video elapsed
	audHave    bool
	audFirstTS uint32
	audLastOut uint32
	audClock   float64
	audVidBase float64 // video elapsed captured when audio anchored (offset correction)
}

// ReclockAudio enables the audio->video re-clock (go2rtc#2303). Off by default;
// opt in with `rtsp: { audio_reclock: true }`. Set from config by internal/rtsp.
var ReclockAudio = false

// reclockAudioToVideo pegs an incoming audio packet's RTP timestamp to VIDEO's
// elapsed time so audio cannot drift away from video (the cause of the
// long-run muxer audio-starvation on these cams). Video packets only update the
// reference; their timestamps are never modified.
func (c *Conn) reclockAudioToVideo(receiver *core.Receiver, packet *rtp.Packet) {
	codec := receiver.Codec
	if codec == nil {
		return
	}
	switch codec.Kind() {
	case core.KindVideo:
		if !c.vidHave {
			c.vidHave = true
			c.vidLastTS = packet.Timestamp
			c.vidClock = float64(codec.ClockRate)
			if c.vidClock <= 0 {
				c.vidClock = 90000
			}
			return
		}
		c.vidTicks += int64(int32(packet.Timestamp - c.vidLastTS)) // wrap-safe signed delta
		c.vidLastTS = packet.Timestamp
		c.vidElapsed = float64(c.vidTicks) / c.vidClock
	case core.KindAudio:
		if !c.vidHave {
			return // no video reference yet — leave audio untouched
		}
		if !c.audHave {
			c.audHave = true
			c.audFirstTS = packet.Timestamp
			c.audClock = float64(codec.ClockRate)
			if c.audClock <= 0 {
				c.audClock = 8000
			}
			// Capture video's elapsed at the moment audio anchors. Audio often
			// starts LATER than video on a connection (reconnects bring video up
			// first); without this, audio elapsed was measured from video's
			// start and jumped forward by the offset, desyncing audio for the
			// whole connection (go2rtc#2303 residual).
			c.audVidBase = c.vidElapsed
			c.audLastOut = packet.Timestamp
			log.Printf("[reclock] %s audio anchored, video was %.2fs in (offset corrected)", c.URL, c.audVidBase)
			return
		}
		out := c.audFirstTS + uint32(int64((c.vidElapsed-c.audVidBase)*c.audClock+0.5))
		if int32(out-c.audLastOut) < 1 {
			out = c.audLastOut + 1 // keep monotonic between video updates
		}
		c.audLastOut = out
		packet.Timestamp = out
	}
}

const (
	ProtoRTSP      = "RTSP/1.0"
	MethodOptions  = "OPTIONS"
	MethodSetup    = "SETUP"
	MethodTeardown = "TEARDOWN"
	MethodDescribe = "DESCRIBE"
	MethodPlay     = "PLAY"
	MethodPause    = "PAUSE"
	MethodAnnounce = "ANNOUNCE"
	MethodRecord   = "RECORD"
)

type State byte

func (s State) String() string {
	switch s {
	case StateNone:
		return "NONE"
	case StateConn:

		return "CONN"
	case StateSetup:
		return MethodSetup
	case StatePlay:
		return MethodPlay
	}
	return strconv.Itoa(int(s))
}

const (
	StateNone State = iota
	StateConn
	StateSetup
	StatePlay
)

func (c *Conn) Handle() (err error) {
	var timeout time.Duration

	var keepaliveDT time.Duration
	var keepaliveTS time.Time

	switch c.mode {
	case core.ModeActiveProducer:
		if c.keepalive > 5 {
			keepaliveDT = time.Duration(c.keepalive-5) * time.Second
		} else {
			keepaliveDT = 25 * time.Second
		}
		keepaliveTS = time.Now().Add(keepaliveDT)

		if c.Timeout == 0 {
			// polling frames from remote RTSP Server (ex Camera)
			timeout = time.Second * 5

			if len(c.Receivers) == 0 {
				// if we only send audio to camera
				// https://github.com/AlexxIT/go2rtc/issues/659
				timeout += keepaliveDT
			}
		} else {
			timeout = time.Second * time.Duration(c.Timeout)
		}

	case core.ModePassiveProducer:
		// polling frames from remote RTSP Client (ex FFmpeg)
		if c.Timeout == 0 {
			timeout = time.Second * 15
		} else {
			timeout = time.Second * time.Duration(c.Timeout)
		}

	case core.ModePassiveConsumer:
		// pushing frames to remote RTSP Client (ex VLC)
		timeout = time.Second * 60

	default:
		return fmt.Errorf("wrong RTSP conn mode: %d", c.mode)
	}

	for c.state != StateNone {
		ts := time.Now()

		if err = c.conn.SetReadDeadline(ts.Add(timeout)); err != nil {
			return
		}

		// we can read:
		// 1. RTP interleaved: `$` + 1B channel number + 2B size
		// 2. RTSP response:   RTSP/1.0 200 OK
		// 3. RTSP request:    OPTIONS ...
		var buf4 []byte // `$` + 1B channel number + 2B size
		buf4, err = c.reader.Peek(4)
		if err != nil {
			return
		}

		var channelID byte
		var size uint16

		if buf4[0] != '$' {
			switch string(buf4) {
			case "RTSP":
				var res *tcp.Response
				if res, err = c.ReadResponse(); err != nil {
					return
				}
				c.Fire(res)
				// for playing backchannel only after OK response on play
				c.playOK = true
				continue

			case "OPTI", "TEAR", "DESC", "SETU", "PLAY", "PAUS", "RECO", "ANNO", "GET_", "SET_":
				var req *tcp.Request
				if req, err = c.ReadRequest(); err != nil {
					return
				}
				c.Fire(req)
				if req.Method == MethodOptions {
					res := &tcp.Response{Request: req}
					if err = c.WriteResponse(res); err != nil {
						return
					}
				}
				continue

			default:
				c.Fire("RTSP wrong input")

				for i := 0; ; i++ {
					// search next start symbol
					if _, err = c.reader.ReadBytes('$'); err != nil {
						return err
					}

					if channelID, err = c.reader.ReadByte(); err != nil {
						return err
					}

					// TODO: better check maximum good channel ID
					if channelID >= 20 {
						continue
					}

					buf4 = make([]byte, 2)
					if _, err = io.ReadFull(c.reader, buf4); err != nil {
						return err
					}

					// check if size good for RTP
					size = binary.BigEndian.Uint16(buf4)
					if size <= 1500 {
						break
					}

					// 10 tries to find good packet
					if i >= 10 {
						return fmt.Errorf("RTSP wrong input")
					}
				}
			}
		} else {
			// hope that the odd channels are always RTCP
			channelID = buf4[1]

			// get data size
			size = binary.BigEndian.Uint16(buf4[2:])

			// skip 4 bytes from c.reader.Peek
			if _, err = c.reader.Discard(4); err != nil {
				return
			}
		}

		// init memory for data
		buf := make([]byte, size)
		if _, err = io.ReadFull(c.reader, buf); err != nil {
			return
		}

		c.Recv += int(size)

		if channelID&1 == 0 {
			packet := &rtp.Packet{}
			if err = packet.Unmarshal(buf); err != nil {
				return
			}

			for _, receiver := range c.Receivers {
				if receiver.ID == channelID {
					if ReclockAudio {
						c.reclockAudioToVideo(receiver, packet)
					}
					receiver.WriteRTP(packet)
					break
				}
			}
		} else {
			msg := &RTCP{Channel: channelID}

			if err = msg.Header.Unmarshal(buf); err != nil {
				continue
			}

			msg.Packets, err = rtcp.Unmarshal(buf)
			if err != nil {
				continue
			}

			c.Fire(msg)
		}

		if keepaliveDT != 0 && ts.After(keepaliveTS) {
			req := &tcp.Request{Method: MethodOptions, URL: c.URL}
			if err = c.WriteRequest(req); err != nil {
				return
			}

			keepaliveTS = ts.Add(keepaliveDT)
		}
	}

	return
}

func (c *Conn) WriteRequest(req *tcp.Request) error {
	if req.Proto == "" {
		req.Proto = ProtoRTSP
	}

	if req.Header == nil {
		req.Header = make(map[string][]string)
	}

	c.sequence++
	// important to send case sensitive CSeq
	// https://github.com/AlexxIT/go2rtc/issues/7
	req.Header["CSeq"] = []string{strconv.Itoa(c.sequence)}

	c.auth.Write(req)

	if c.session != "" {
		req.Header.Set("Session", c.session)
	}

	if req.Body != nil {
		val := strconv.Itoa(len(req.Body))
		req.Header.Set("Content-Length", val)
	}

	c.Fire(req)

	if err := c.conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return err
	}

	return req.Write(c.conn)
}

func (c *Conn) ReadRequest() (*tcp.Request, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(Timeout)); err != nil {
		return nil, err
	}
	return tcp.ReadRequest(c.reader)
}

func (c *Conn) WriteResponse(res *tcp.Response) error {
	if res.Proto == "" {
		res.Proto = ProtoRTSP
	}

	if res.Status == "" {
		res.Status = "200 OK"
	}

	if res.Header == nil {
		res.Header = make(map[string][]string)
	}

	if res.Request != nil && res.Request.Header != nil {
		seq := res.Request.Header.Get("CSeq")
		if seq != "" {
			res.Header.Set("CSeq", seq)
		}
	}

	if c.session != "" {
		if res.Request != nil && res.Request.Method == MethodSetup {
			res.Header.Set("Session", c.session+";timeout=60")
		} else {
			res.Header.Set("Session", c.session)
		}
	}

	if res.Body != nil {
		val := strconv.Itoa(len(res.Body))
		res.Header.Set("Content-Length", val)
	}

	c.Fire(res)

	if err := c.conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return err
	}

	return res.Write(c.conn)
}

func (c *Conn) ReadResponse() (*tcp.Response, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(Timeout)); err != nil {
		return nil, err
	}
	return tcp.ReadResponse(c.reader)
}
