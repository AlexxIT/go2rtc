package rtsp

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

func NewClient(uri string) *Conn {
	return &Conn{uri: uri}
}

func (c *Conn) Dial() (err error) {
	if c.URL, err = url.Parse(c.uri); err != nil {
		return
	}

	if strings.IndexByte(c.URL.Host, ':') < 0 {
		c.URL.Host += ":554"
	}

	// remove UserInfo from URL
	c.auth = tcp.NewAuth(c.URL.User)
	c.URL.User = nil

	c.conn, err = net.DialTimeout("tcp", c.URL.Host, time.Second*5)
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
	c.state = StateConn

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
	// important to send case sensitive CSeq
	// https://github.com/AlexxIT/go2rtc/issues/7
	req.Header["CSeq"] = []string{strconv.Itoa(c.sequence)}

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
			if c.auth.ReadNone(res) {
				return c.Do(req)
			}
			return nil, errors.New("user/pass not provided")
		case tcp.AuthUnknown:
			if c.auth.Read(res) {
				return c.Do(req)
			}
		default:
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
		c.URL, err = urlParse(val)
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
			"Accept": {"application/sdp"},
		},
	}

	if c.Backchannel {
		req.Header.Set("Require", "www.onvif.org/ver20/backchannel")
	}

	if c.UserAgent != "" {
		// this camera will answer with 401 on DESCRIBE without User-Agent
		// https://github.com/AlexxIT/go2rtc/issues/235
		req.Header.Set("User-Agent", c.UserAgent)
	}

	res, err := c.Do(req)
	if err != nil {
		return err
	}

	if val := res.Header.Get("Content-Base"); val != "" {
		c.URL, err = urlParse(val)
		if err != nil {
			return err
		}
	}

	c.Medias, err = UnmarshalSDP(res.Body)
	if err != nil {
		return err
	}

	c.mode = core.ModeActiveProducer

	return nil
}

func (c *Conn) Announce() (err error) {
	req := &tcp.Request{
		Method: MethodAnnounce,
		URL:    c.URL,
		Header: map[string][]string{
			"Content-Type": {"application/sdp"},
		},
	}

	req.Body, err = core.MarshalSDP(c.SessionName, c.Medias)
	if err != nil {
		return err
	}

	res, err := c.Do(req)

	_ = res

	return
}

func (c *Conn) Setup() error {
	for _, media := range c.Medias {
		_, err := c.SetupMedia(media, true)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Conn) SetupMedia(media *core.Media, first bool) (byte, error) {
	// TODO: rewrite recoonection and first flag
	if first {
		c.stateMu.Lock()
		defer c.stateMu.Unlock()
	}

	if c.state != StateConn && c.state != StateSetup {
		return 0, fmt.Errorf("RTSP SETUP from wrong state: %s", c.state)
	}

	var transport string

	// try to use media position as channel number
	for i, m := range c.Medias {
		if m.ID == media.ID {
			transport = fmt.Sprintf(
				// i   - RTP (data channel)
				// i+1 - RTCP (control channel)
				"RTP/AVP/TCP;unicast;interleaved=%d-%d", i*2, i*2+1,
			)
			break
		}
	}

	if transport == "" {
		return 0, fmt.Errorf("wrong media: %v", media)
	}

	rawURL := media.ID // control
	if !strings.Contains(rawURL, "://") {
		rawURL = c.URL.String()
		if !strings.HasSuffix(rawURL, "/") {
			rawURL += "/"
		}
		rawURL += media.ID
	}
	trackURL, err := urlParse(rawURL)
	if err != nil {
		return 0, err
	}

	req := &tcp.Request{
		Method: MethodSetup,
		URL:    trackURL,
		Header: map[string][]string{
			"Transport": {transport},
		},
	}

	var res *tcp.Response
	res, err = c.Do(req)
	if err != nil {
		// some Dahua/Amcrest cameras fail here because two simultaneous
		// backchannel connections
		if c.Backchannel {
			c.Close()
			c.Backchannel = false
			if err := c.Dial(); err != nil {
				return 0, err
			}
			if err := c.Describe(); err != nil {
				return 0, err
			}

			for _, newMedia := range c.Medias {
				if newMedia.ID == media.ID {
					return c.SetupMedia(newMedia, false)
				}
			}
		}

		return 0, err
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
	// Transport: RTP/AVP/TCP;unicast;destination=192.168.1.111;source=192.168.1.222;interleaved=0
	// Transport: RTP/AVP/TCP;ssrc=22345682;interleaved=0-1
	transport = res.Header.Get("Transport")
	if !strings.HasPrefix(transport, "RTP/AVP/TCP;") {
		// Escam Q6 has a bug:
		// Transport: RTP/AVP;unicast;destination=192.168.1.111;source=192.168.1.222;interleaved=0-1
		if !strings.Contains(transport, ";interleaved=") {
			return 0, fmt.Errorf("wrong transport: %s", transport)
		}
	}

	c.state = StateSetup

	channel := core.Between(transport, "interleaved=", "-")
	i, err := strconv.Atoi(channel)
	if err != nil {
		return 0, err
	}

	return byte(i), nil
}

func (c *Conn) Play() (err error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.state != StateSetup {
		return fmt.Errorf("RTSP PLAY from wrong state: %s", c.state)
	}

	req := &tcp.Request{Method: MethodPlay, URL: c.URL}
	if err = c.Request(req); err == nil {
		c.state = StatePlay
	}

	return
}

func (c *Conn) Teardown() (err error) {
	// allow TEARDOWN from any state (ex. ANNOUNCE > SETUP)
	req := &tcp.Request{Method: MethodTeardown, URL: c.URL}
	return c.Request(req)
}

func (c *Conn) Close() error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.state == StateNone {
		return nil
	}

	_ = c.Teardown()
	c.state = StateNone
	return c.conn.Close()
}
