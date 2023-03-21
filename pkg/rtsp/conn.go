package rtsp

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"io"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"
)

type Conn struct {
	core.Listener

	// public

	Backchannel bool
	SessionName string

	Medias    []*core.Media
	Session   string
	UserAgent string
	URL       *url.URL

	// internal

	auth     *tcp.Auth
	conn     net.Conn
	mode     core.Mode
	state    State
	stateMu  sync.Mutex
	reader   *bufio.Reader
	sequence int
	uri      string

	receivers []*core.Receiver
	senders   []*core.Sender

	// stats

	recv int
	send int
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
		return "SETUP"
	case StatePlay:
		return "PLAY"
	case StateHandle:
		return "HANDLE"
	}
	return strconv.Itoa(int(s))
}

const (
	StateNone State = iota
	StateConn
	StateSetup
	StatePlay
	StateHandle
)

func (c *Conn) Handle() (err error) {
	c.stateMu.Lock()

	switch c.state {
	case StateNone: // Close after PLAY and before Handle is OK (because SETUP after PLAY)
	case StatePlay:
		c.state = StateHandle
	default:
		err = fmt.Errorf("RTSP HANDLE from wrong state: %s", c.state)

		c.state = StateNone
		_ = c.conn.Close()
	}

	ok := c.state == StateHandle

	c.stateMu.Unlock()

	if !ok {
		return
	}

	var timeout time.Duration

	switch c.mode {
	case core.ModeActiveProducer:
		// polling frames from remote RTSP Server (ex Camera)
		go c.keepalive()

		if len(c.receivers) > 0 {
			// if we receiving video/audio from camera
			timeout = time.Second * 5
		} else {
			// if we only send audio to camera
			timeout = time.Second * 30
		}

	case core.ModePassiveProducer:
		// polling frames from remote RTSP Client (ex FFmpeg)
		timeout = time.Second * 15

	case core.ModePassiveConsumer:
		// pushing frames to remote RTSP Client (ex VLC)
		timeout = time.Second * 60

	default:
		return fmt.Errorf("wrong RTSP conn mode: %d", c.mode)
	}

	for c.state != StateNone {
		if err = c.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
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
				if res, err = tcp.ReadResponse(c.reader); err != nil {
					return
				}
				c.Fire(res)
				continue

			case "OPTI", "TEAR", "DESC", "SETU", "PLAY", "PAUS", "RECO", "ANNO", "GET_", "SET_":
				var req *tcp.Request
				if req, err = tcp.ReadRequest(c.reader); err != nil {
					return
				}
				c.Fire(req)
				continue

			default:
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

				c.Fire("RTSP wrong input")
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

		c.recv += int(size)

		if channelID&1 == 0 {
			packet := &rtp.Packet{}
			if err = packet.Unmarshal(buf); err != nil {
				return
			}

			for _, receiver := range c.receivers {
				if receiver.ID == channelID {
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
	}

	return
}

func (c *Conn) keepalive() {
	// TODO: rewrite to RTCP
	req := &tcp.Request{Method: MethodOptions, URL: c.URL}
	for {
		time.Sleep(time.Second * 25)
		if c.state == StateNone {
			return
		}
		if err := c.Request(req); err != nil {
			return
		}
	}
}
