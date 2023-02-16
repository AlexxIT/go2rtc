package rtsp

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"net"
	"net/url"
	"strings"
)

func NewServer(conn net.Conn) *Conn {
	c := new(Conn)
	c.conn = conn
	c.mode = ModeServerUnknown
	c.reader = bufio.NewReader(conn)
	return c
}

func (c *Conn) Auth(username, password string) {
	info := url.UserPassword(username, password)
	c.auth = tcp.NewAuth(info)
}

const transport = "RTP/AVP/TCP;unicast;interleaved="

func (c *Conn) Accept() error {
	for {
		req, err := tcp.ReadRequest(c.reader)
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
			if err = c.Response(res); err != nil {
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
			if err = c.Response(res); err != nil {
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
			c.channels = map[byte]*streamer.Track{}
			for i, media := range c.Medias {
				track := streamer.NewTrack(media.Codecs[0], media.Direction)
				c.tracks = append(c.tracks, track)
				c.channels[byte(i<<1)] = track
			}

			c.mode = ModeServerProducer
			c.Fire(MethodAnnounce)

			res := &tcp.Response{Request: req}
			if err = c.Response(res); err != nil {
				return err
			}

		case MethodDescribe:
			c.mode = ModeServerConsumer
			c.Fire(MethodDescribe)

			if c.tracks == nil {
				res := &tcp.Response{
					Status:  "404 Not Found",
					Request: req,
				}
				return c.Response(res)
			}

			res := &tcp.Response{
				Header: map[string][]string{
					"Content-Type": {"application/sdp"},
				},
				Request: req,
			}

			// convert tracks to real output medias medias
			var medias []*streamer.Media
			for _, track := range c.tracks {
				media := &streamer.Media{
					Kind:      streamer.GetKind(track.Codec.Name),
					Direction: streamer.DirectionSendonly,
					Codecs:    []*streamer.Codec{track.Codec},
				}
				medias = append(medias, media)
			}

			res.Body, err = streamer.MarshalSDP(c.SessionName, medias)
			if err != nil {
				return err
			}

			if err = c.Response(res); err != nil {
				return err
			}

		case MethodSetup:
			tr := req.Header.Get("Transport")

			res := &tcp.Response{
				Header:  map[string][]string{},
				Request: req,
			}

			if strings.HasPrefix(tr, transport) {
				c.Session = "1" // TODO: fixme
				c.state = StateSetup
				res.Header.Set("Transport", tr[:len(transport)+3])
			} else {
				res.Status = "461 Unsupported transport"
			}

			if err = c.Response(res); err != nil {
				return err
			}

		case MethodRecord, MethodPlay:
			res := &tcp.Response{Request: req}
			if err = c.Response(res); err == nil {
				c.state = StatePlay
			}
			return err

		case MethodTeardown:
			res := &tcp.Response{Request: req}
			_ = c.Response(res)
			c.state = StateNone
			return c.conn.Close()

		default:
			return fmt.Errorf("unsupported method: %s", req.Method)
		}
	}
}
