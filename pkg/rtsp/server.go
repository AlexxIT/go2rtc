package rtsp

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

func NewServer(conn net.Conn) *Conn {
	return &Conn{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "rtsp",
			Protocol:   "rtsp+tcp",
			RemoteAddr: conn.RemoteAddr().String(),
		},
		conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

func (c *Conn) Auth(username, password string) {
	info := url.UserPassword(username, password)
	c.auth = tcp.NewAuth(info)
}

func (c *Conn) Accept() error {
	for {
		req, err := c.ReadRequest()
		if err != nil {
			return err
		}

		if c.URL == nil {
			c.URL = req.URL
			c.UserAgent = req.Header.Get("User-Agent")
		}

		c.Fire(req)

		if !c.auth.Validate(req) {
			res := &tcp.Response{
				Status:  "401 Unauthorized",
				Header:  map[string][]string{"Www-Authenticate": {`Basic realm="go2rtc"`}},
				Request: req,
			}
			if err = c.WriteResponse(res); err != nil {
				return err
			}
			continue
		}

		// Receiver: OPTIONS > DESCRIBE > SETUP... > PLAY > TEARDOWN
		// Sender: OPTIONS > ANNOUNCE > SETUP... > RECORD > TEARDOWN
		switch req.Method {
		case MethodOptions:
			res := &tcp.Response{
				Header: map[string][]string{
					"Public": {"OPTIONS, SETUP, TEARDOWN, DESCRIBE, PLAY, PAUSE, ANNOUNCE, RECORD"},
				},
				Request: req,
			}
			if err = c.WriteResponse(res); err != nil {
				return err
			}

		case MethodAnnounce:
			if req.Header.Get("Content-Type") != "application/sdp" {
				return errors.New("wrong content type")
			}

			c.SDP = string(req.Body) // for info

			c.Medias, err = UnmarshalSDP(req.Body)
			if err != nil {
				return err
			}

			// TODO: fix someday...
			for i, media := range c.Medias {
				track := core.NewReceiver(media, media.Codecs[0])
				track.ID = byte(i * 2)
				c.Receivers = append(c.Receivers, track)
			}

			c.mode = core.ModePassiveProducer
			c.Fire(MethodAnnounce)

			res := &tcp.Response{Request: req}
			if err = c.WriteResponse(res); err != nil {
				return err
			}

		case MethodDescribe:
			c.mode = core.ModePassiveConsumer
			c.Fire(MethodDescribe)

			if c.Senders == nil {
				res := &tcp.Response{
					Status:  "404 Not Found",
					Request: req,
				}
				return c.WriteResponse(res)
			}

			res := &tcp.Response{
				Header: map[string][]string{
					"Content-Type": {"application/sdp"},
				},
				Request: req,
			}

			// convert tracks to real output medias medias
			var medias []*core.Media
			for i, track := range c.Senders {
				media := &core.Media{
					Kind:      core.GetKind(track.Codec.Name),
					Direction: core.DirectionRecvonly,
					Codecs:    []*core.Codec{track.Codec},
					ID:        "trackID=" + strconv.Itoa(i),
				}
				medias = append(medias, media)
			}

			res.Body, err = core.MarshalSDP(c.SessionName, medias)
			if err != nil {
				return err
			}

			c.SDP = string(res.Body) // for info

			if err = c.WriteResponse(res); err != nil {
				return err
			}

		case MethodSetup:
			tr := req.Header.Get("Transport")

			res := &tcp.Response{
				Header:  map[string][]string{},
				Request: req,
			}

			const transport = "RTP/AVP/TCP;unicast;interleaved="
			if strings.HasPrefix(tr, transport) {
				c.session = core.RandString(8, 10)
				c.state = StateSetup

				if c.mode == core.ModePassiveConsumer {
					if i := reqTrackID(req); i >= 0 && i < len(c.Senders) {
						// mark sender as SETUP
						c.Senders[i].Media.ID = MethodSetup
						tr = fmt.Sprintf("RTP/AVP/TCP;unicast;interleaved=%d-%d", i*2, i*2+1)
						res.Header.Set("Transport", tr)
					} else {
						res.Status = "400 Bad Request"
					}
				} else {
					res.Header.Set("Transport", tr[:len(transport)+3])
				}
			} else {
				res.Status = "461 Unsupported transport"
			}

			if err = c.WriteResponse(res); err != nil {
				return err
			}

		case MethodRecord, MethodPlay:
			if c.mode == core.ModePassiveConsumer {
				// stop unconfigured senders
				for _, track := range c.Senders {
					if track.Media.ID != MethodSetup {
						track.Close()
					}
				}
			}

			res := &tcp.Response{Request: req}
			err = c.WriteResponse(res)
			c.playOK = true
			return err

		case MethodTeardown:
			res := &tcp.Response{Request: req}
			_ = c.WriteResponse(res)
			c.state = StateNone
			return c.conn.Close()

		default:
			return fmt.Errorf("unsupported method: %s", req.Method)
		}
	}
}

func reqTrackID(req *tcp.Request) int {
	var s string
	if req.URL.RawQuery != "" {
		s = req.URL.RawQuery
	} else {
		s = req.URL.Path
	}
	if i := strings.LastIndexByte(s, '='); i > 0 {
		if i, err := strconv.Atoi(s[i+1:]); err == nil {
			return i
		}
	}
	return -1
}
