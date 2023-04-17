package rtsp

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func NewServer(conn net.Conn) *Conn {
	c := new(Conn)
	c.conn = conn
	c.reader = bufio.NewReader(conn)
	return c
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
				Status: "401 Unauthorized",
				Header: map[string][]string{"Www-Authenticate": {`Basic realm="go2rtc"`}},
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

			c.Medias, err = UnmarshalSDP(req.Body)
			if err != nil {
				return err
			}

			// TODO: fix someday...
			for i, media := range c.Medias {
				track := core.NewReceiver(media, media.Codecs[0])
				track.ID = byte(i * 2)
				c.receivers = append(c.receivers, track)
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

			if c.senders == nil {
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
			for i, track := range c.senders {
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
				res.Header.Set("Transport", tr[:len(transport)+3])
			} else {
				res.Status = "461 Unsupported transport"
			}

			if err = c.WriteResponse(res); err != nil {
				return err
			}

		case MethodRecord, MethodPlay:
			res := &tcp.Response{Request: req}
			return c.WriteResponse(res)

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
