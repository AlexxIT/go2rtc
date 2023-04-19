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

var Timeout = time.Second * 5

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

	// remove UserInfo from URL
	c.auth = tcp.NewAuth(c.URL.User)
	c.URL.User = nil

	c.reader = bufio.NewReader(c.conn)
	c.session = ""
	c.state = StateConn

	return nil
}

// Do send WriteRequest and receive and process WriteResponse
func (c *Conn) Do(req *tcp.Request) (*tcp.Response, error) {
	if err := c.WriteRequest(req); err != nil {
		return nil, err
	}

	res, err := c.ReadResponse()
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

	medias, err := UnmarshalSDP(res.Body)
	if err != nil {
		return err
	}

	// TODO: rewrite more smart
	if c.Medias == nil {
		c.Medias = medias
	} else if len(c.Medias) > len(medias) {
		c.Medias = c.Medias[:len(medias)]
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

func (c *Conn) SetupMedia(media *core.Media) (byte, error) {
	var transport string

	// try to use media position as channel number
	for i, m := range c.Medias {
		if m.Equal(media) {
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

	res, err := c.Do(req)
	if err != nil {
		// some Dahua/Amcrest cameras fail here because two simultaneous
		// backchannel connections
		if c.Backchannel {
			c.Backchannel = false
			if err = c.Reconnect(); err != nil {
				return 0, err
			}
			return c.SetupMedia(media)
		}

		return 0, err
	}

	if c.session == "" {
		// Session: 216525287999;timeout=60
		if s := res.Header.Get("Session"); s != "" {
			c.session, s, _ = strings.Cut(s, ";timeout=")
			if s != "" {
				c.keepalive, _ = strconv.Atoi(s)
			}
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

	channel := core.Between(transport, "interleaved=", "-")
	i, err := strconv.Atoi(channel)
	if err != nil {
		return 0, err
	}

	return byte(i), nil
}

func (c *Conn) Play() (err error) {
	req := &tcp.Request{Method: MethodPlay, URL: c.URL}
	return c.WriteRequest(req)
}

func (c *Conn) Teardown() (err error) {
	// allow TEARDOWN from any state (ex. ANNOUNCE > SETUP)
	req := &tcp.Request{Method: MethodTeardown, URL: c.URL}
	return c.WriteRequest(req)
}

func (c *Conn) Close() error {
	if c.mode == core.ModeActiveProducer {
		_ = c.Teardown()
	}
	return c.conn.Close()
}
