package rtsp

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

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

type Mode byte

const (
	ModeUnknown Mode = iota
	ModeClientProducer
	ModeServerUnknown
	ModeServerProducer
	ModeServerConsumer
)

type Conn struct {
	streamer.Element

	// public

	Medias    []*streamer.Media
	Session   string
	UserAgent string
	URL       *url.URL

	// internal

	auth     *tcp.Auth
	conn     net.Conn
	reader   *bufio.Reader
	sequence int

	mode Mode

	tracks   []*streamer.Track
	channels map[byte]*streamer.Track

	// stats

	receive int
	send    int
}

func NewClient(uri string) (*Conn, error) {
	var err error

	c := new(Conn)
	c.URL, err = url.Parse(uri)
	if err != nil {
		return nil, err
	}

	if strings.IndexByte(c.URL.Host, ':') < 0 {
		c.URL.Host += ":554"
	}

	// remove UserInfo from URL
	c.auth = tcp.NewAuth(c.URL.User)
	c.mode = ModeClientProducer
	c.URL.User = nil

	return c, nil
}

func NewServer(conn net.Conn) *Conn {
	c := new(Conn)
	c.conn = conn
	c.mode = ModeServerUnknown
	c.reader = bufio.NewReader(conn)
	return c
}

func (c *Conn) Dial() (err error) {
	//if c.state != StateClientInit {
	//	panic("wrong state")
	//}

	c.conn, err = net.DialTimeout(
		"tcp", c.URL.Host, 10*time.Second,
	)
	if err != nil {
		return
	}

	var tlsConf *tls.Config
	switch c.URL.Scheme {
	case "rtsps":
		tlsConf = &tls.Config{ServerName: c.URL.Hostname()}
	case "rtspx":
		c.URL.Scheme = "rtsps"
		tlsConf = &tls.Config{InsecureSkipVerify: true}
	}
	if tlsConf != nil {
		tlsConn := tls.Client(c.conn, tlsConf)
		if err = tlsConn.Handshake(); err != nil {
			return err
		}
		c.conn = tlsConn
	}

	c.reader = bufio.NewReader(c.conn)

	return nil
}

// Request sends only Request
func (c *Conn) Request(req *tcp.Request) error {
	if req.Proto == "" {
		req.Proto = ProtoRTSP
	}

	if req.Header == nil {
		req.Header = make(map[string][]string)
	}

	c.sequence++
	req.Header.Set("CSeq", strconv.Itoa(c.sequence))

	c.auth.Write(req)

	if c.Session != "" {
		req.Header.Set("Session", c.Session)
	}

	if req.Body != nil {
		val := strconv.Itoa(len(req.Body))
		req.Header.Set("Content-Length", val)
	}

	c.Fire(req)

	return req.Write(c.conn)
}

// Do send Request and receive and process Response
func (c *Conn) Do(req *tcp.Request) (*tcp.Response, error) {
	if err := c.Request(req); err != nil {
		return nil, err
	}

	res, err := tcp.ReadResponse(c.reader)
	if err != nil {
		return nil, err
	}

	c.Fire(res)

	if res.StatusCode == http.StatusUnauthorized {
		switch c.auth.Method {
		case tcp.AuthNone:
			return nil, errors.New("user/pass not provided")
		case tcp.AuthUnknown:
			if c.auth.Read(res) {
				return c.Do(req)
			}
		case tcp.AuthBasic, tcp.AuthDigest:
			return nil, errors.New("wrong user/pass")
		}
	}

	if res.StatusCode != http.StatusOK {
		return res, fmt.Errorf("wrong response on %s", req.Method)
	}

	return res, nil
}

func (c *Conn) Response(res *tcp.Response) error {
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

	if c.Session != "" {
		res.Header.Set("Session", c.Session)
	}

	if res.Body != nil {
		val := strconv.Itoa(len(res.Body))
		res.Header.Set("Content-Length", val)
	}

	c.Fire(res)

	return res.Write(c.conn)
}

func (c *Conn) Options() error {
	req := &tcp.Request{Method: MethodOptions, URL: c.URL}

	res, err := c.Do(req)
	if err != nil {
		return err
	}

	if val := res.Header.Get("Content-Base"); val != "" {
		c.URL, err = url.Parse(val)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Conn) Describe() error {
	// 5.3 Back channel connection
	// https://www.onvif.org/specs/stream/ONVIF-Streaming-Spec.pdf
	req := &tcp.Request{
		Method: MethodDescribe,
		URL:    c.URL,
		Header: map[string][]string{
			"Accept":  {"application/sdp"},
			"Require": {"www.onvif.org/ver20/backchannel"},
		},
	}

	res, err := c.Do(req)
	if err != nil {
		if res != nil {
			// if we have answer - give second chanse without onvif header
			req.Header.Del("Require")
			res, err = c.Do(req)
		}
		if err != nil {
			return err
		}
	}

	if val := res.Header.Get("Content-Base"); val != "" {
		c.URL, err = url.Parse(val)
		if err != nil {
			return err
		}
	}

	// fix bug in Sonoff camera SDP "o=- 1 1 IN IP4 rom t_rtsplin"
	// TODO: make some universal fix
	if i := bytes.Index(res.Body, []byte("rom t_rtsplin")); i > 0 {
		res.Body[i+3] = '_'
	}

	c.Medias, err = streamer.UnmarshalRTSPSDP(res.Body)
	if err != nil {
		return err
	}

	c.mode = ModeClientProducer

	return nil
}

//func (c *Conn) Announce() (err error) {
//	req := &tcp.Request{
//		Method: MethodAnnounce,
//		URL:    c.URL,
//		Header: map[string][]string{
//			"Content-Type": {"application/sdp"},
//		},
//	}
//
//	//req.Body, err = c.sdp.Marshal()
//	if err != nil {
//		return
//	}
//
//	_, err = c.Do(req)
//
//	return
//}

func (c *Conn) Setup() error {
	for _, media := range c.Medias {
		_, err := c.SetupMedia(media, media.Codecs[0])
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Conn) SetupMedia(
	media *streamer.Media, codec *streamer.Codec,
) (*streamer.Track, error) {
	ch := c.GetChannel(media)
	if ch < 0 {
		return nil, fmt.Errorf("wrong media: %v", media)
	}

	trackURL, err := url.Parse(media.Control)
	if err != nil {
		return nil, err
	}
	trackURL = c.URL.ResolveReference(trackURL)

	req := &tcp.Request{
		Method: MethodSetup,
		URL:    trackURL,
		Header: map[string][]string{
			"Transport": {fmt.Sprintf(
				// i   - RTP (data channel)
				// i+1 - RTCP (control channel)
				"RTP/AVP/TCP;unicast;interleaved=%d-%d", ch*2, ch*2+1,
			)},
		},
	}

	var res *tcp.Response
	res, err = c.Do(req)
	if err != nil {
		return nil, err
	}

	if c.Session == "" {
		// Session: 216525287999;timeout=60
		if s := res.Header.Get("Session"); s != "" {
			if j := strings.IndexByte(s, ';'); j > 0 {
				s = s[:j]
			}
			c.Session = s
		}
	}

	// we send our `interleaved`, but camera can answer with another

	// Transport: RTP/AVP/TCP;unicast;interleaved=10-11;ssrc=10117CB7
	// Transport: RTP/AVP/TCP;unicast;destination=192.168.1.123;source=192.168.10.12;interleaved=0
	s := res.Header.Get("Transport")
	// TODO: rewrite
	if !strings.HasPrefix(s, "RTP/AVP/TCP;unicast") {
		return nil, fmt.Errorf("wrong transport: %s", s)
	}

	i := strings.Index(s, "interleaved=")
	if i < 0 {
		return nil, fmt.Errorf("wrong transport: %s", s)
	}

	s = s[i+len("interleaved="):]
	i = strings.IndexAny(s, "-;")
	if i > 0 {
		s = s[:i]
	}

	ch, err = strconv.Atoi(s)
	if err != nil {
		return nil, err
	}

	track := &streamer.Track{
		Codec: codec, Direction: media.Direction,
	}

	switch track.Direction {
	case streamer.DirectionSendonly:
		if c.channels == nil {
			c.channels = make(map[byte]*streamer.Track)
		}
		c.channels[byte(ch)] = track

	case streamer.DirectionRecvonly:
		track = c.bindTrack(track, byte(ch), codec.PayloadType)
	}

	c.tracks = append(c.tracks, track)

	return track, nil
}

func (c *Conn) Play() (err error) {
	req := &tcp.Request{Method: MethodPlay, URL: c.URL}
	return c.Request(req)
}

func (c *Conn) Teardown() (err error) {
	//if c.state != StateClientPlay {
	//	panic("wrong state")
	//}

	req := &tcp.Request{Method: MethodTeardown, URL: c.URL}
	return c.Request(req)
}

func (c *Conn) Close() error {
	if c.conn == nil {
		return nil
	}
	if err := c.Teardown(); err != nil {
		return err
	}
	conn := c.conn
	c.conn = nil
	return conn.Close()
}

const transport = "RTP/AVP/TCP;unicast;interleaved="

func (c *Conn) Accept() error {
	//if c.state != StateServerInit {
	//	panic("wrong state")
	//}

	for {
		req, err := tcp.ReadRequest(c.reader)
		if err != nil {
			return err
		}

		c.Fire(req)

		// Receiver: OPTIONS > DESCRIBE > SETUP... > PLAY > TEARDOWN
		// Sender: OPTIONS > ANNOUNCE > SETUP... > RECORD > TEARDOWN
		switch req.Method {
		case MethodOptions:
			c.URL = req.URL
			c.UserAgent = req.Header.Get("User-Agent")

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

			c.Medias, err = streamer.UnmarshalRTSPSDP(req.Body)
			if err != nil {
				return err
			}

			// TODO: fix someday...
			c.channels = map[byte]*streamer.Track{}
			for i, media := range c.Medias {
				track := &streamer.Track{
					Codec: media.Codecs[0], Direction: media.Direction,
				}
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

			res.Body, err = streamer.MarshalSDP(medias)
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

			if tr[:len(transport)] == transport {
				c.Session = "1" // TODO: fixme
				res.Header.Set("Transport", tr[:len(transport)+3])
			} else {
				res.Status = "461 Unsupported transport"
			}

			if err = c.Response(res); err != nil {
				return err
			}

		case MethodRecord, MethodPlay:
			res := &tcp.Response{Request: req}
			return c.Response(res)

		default:
			return fmt.Errorf("unsupported method: %s", req.Method)
		}
	}
}

func (c *Conn) Handle() (err error) {
	defer func() {
		if c.conn == nil {
			err = nil
		}
		//c.Fire(streamer.StateNull)
	}()

	//c.Fire(streamer.StatePlaying)

	for {
		// we can read:
		// 1. RTP interleaved: `$` + 1B channel number + 2B size
		// 2. RTSP response:   RTSP/1.0 200 OK
		// 3. RTSP request:    OPTIONS ...
		var buf4 []byte // `$` + 1B channel number + 2B size
		buf4, err = c.reader.Peek(4)
		if err != nil {
			return
		}

		if buf4[0] != '$' {
			if string(buf4) == "RTSP" {
				var res *tcp.Response
				res, err = tcp.ReadResponse(c.reader)
				if err != nil {
					return
				}

				c.Fire(res)
			} else {
				var req *tcp.Request
				req, err = tcp.ReadRequest(c.reader)
				if err != nil {
					return
				}

				c.Fire(req)
			}
			continue
		}

		// hope that the odd channels are always RTCP
		channelID := buf4[1]

		// get data size
		size := int(binary.BigEndian.Uint16(buf4[2:]))

		if _, err = c.reader.Discard(4); err != nil {
			return
		}

		// init memory for data
		buf := make([]byte, size)
		if _, err = io.ReadFull(c.reader, buf); err != nil {
			return
		}

		c.receive += size

		if channelID&1 == 0 {
			packet := &rtp.Packet{}
			if err = packet.Unmarshal(buf); err != nil {
				return errors.New("wrong RTP data")
			}

			track := c.channels[channelID]
			if track != nil {
				_ = track.WriteRTP(packet)
				//return fmt.Errorf("wrong channelID: %d", channelID)
			} else {
				panic("wrong channelID")
			}
		} else {
			msg := &RTCP{Channel: channelID}

			if err = msg.Header.Unmarshal(buf); err != nil {
				return errors.New("wrong RTCP data")
			}

			msg.Packets, err = rtcp.Unmarshal(buf)
			if err != nil {
				return errors.New("wrong RTCP data")
			}

			c.Fire(msg)
		}
	}
}

func (c *Conn) GetChannel(media *streamer.Media) int {
	for i, m := range c.Medias {
		if m == media {
			return i
		}
	}
	return -1
}

func (c *Conn) bindTrack(
	track *streamer.Track, channel uint8, payloadType uint8,
) *streamer.Track {
	push := func(packet *rtp.Packet) error {
		if c.conn == nil {
			return nil
		}
		packet.Header.PayloadType = payloadType
		//packet.Header.PayloadType = 100
		//packet.Header.PayloadType = 8
		//packet.Header.PayloadType = 106

		size := packet.MarshalSize()

		data := make([]byte, 4+size)
		data[0] = '$'
		data[1] = channel
		//data[1] = 10
		binary.BigEndian.PutUint16(data[2:], uint16(size))

		if _, err := packet.MarshalTo(data[4:]); err != nil {
			return nil
		}

		if _, err := c.conn.Write(data); err != nil {
			return err
		}

		c.send += size

		return nil
	}

	if h264.IsAVC(track.Codec) {
		wrapper := h264.RTPPay(1500)
		push = wrapper(push)
	}

	return track.Bind(push)
}

type RTCP struct {
	Channel byte
	Header  rtcp.Header
	Packets []rtcp.Packet
}

func between(s, sub1, sub2 string) (res string, ok1 bool, ok2 bool) {
	i := strings.Index(s, sub1)
	if i >= 0 {
		ok1 = true
		s = s[i+len(sub1):]
	}

	i = strings.Index(s, sub2)
	if i >= 0 {
		return s[:i], ok1, true
	}

	return s, ok1, false
}
