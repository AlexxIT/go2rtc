package rtsp

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/tcp/websocket"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

var Timeout = time.Second * 5

func NewClient(uri string) *Conn {
	return &Conn{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "rtsp",
		},
		uri: uri,
	}
}

func (c *Conn) Dial() (err error) {
	if c.URL, err = url.Parse(c.uri); err != nil {
		return
	}

	var conn net.Conn

	switch c.Transport {
	case "", "tcp", "udp":
		var timeout time.Duration
		if c.Timeout != 0 {
			timeout = time.Second * time.Duration(c.Timeout)
		} else {
			timeout = core.ConnDialTimeout
		}
		conn, err = tcp.Dial(c.URL, timeout)

		if c.Transport != "udp" {
			c.Protocol = "rtsp+tcp"
		} else {
			c.Protocol = "rtsp+udp"
		}
	default:
		conn, err = websocket.Dial(c.Transport)
		c.Protocol = "ws"
	}
	if err != nil {
		return
	}

	// remove UserInfo from URL
	c.auth = tcp.NewAuth(c.URL.User)
	c.URL.User = nil

	c.conn = conn
	c.reader = bufio.NewReaderSize(conn, core.BufferSize)
	c.session = ""
	c.sequence = 0
	c.state = StateConn

	c.udpConn = nil
	c.udpAddr = nil

	c.Connection.RemoteAddr = conn.RemoteAddr().String()
	c.Connection.Transport = conn
	c.Connection.URL = c.uri

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

	switch res.StatusCode {
	case http.StatusOK:
		return res, nil

	case http.StatusMovedPermanently, http.StatusFound:
		rawURL := res.Header.Get("Location")

		var u *url.URL
		if u, err = url.Parse(rawURL); err != nil {
			return nil, err
		}

		if u.User == nil {
			u.User = c.auth.UserInfo() // restore auth if we don't have it in the new URL
		}

		c.uri = u.String() // so auth will be saved on reconnect

		_ = c.conn.Close()

		if err = c.Dial(); err != nil {
			return nil, err
		}

		req.URL = c.URL // because path was changed

		return c.Do(req)

	case http.StatusUnauthorized:
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

	return res, fmt.Errorf("wrong response on %s", req.Method)
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

	c.SDP = string(res.Body) // for info

	medias, err := UnmarshalSDP(res.Body)
	if err != nil {
		return err
	}

	if c.Media != "" {
		clone := make([]*core.Media, 0, len(medias))
		for _, media := range medias {
			if strings.Contains(c.Media, media.Kind) {
				clone = append(clone, media)
			}
		}
		medias = clone
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

	_, err = c.Do(req)
	return
}

func (c *Conn) Record() (err error) {
	req := &tcp.Request{
		Method: MethodRecord,
		URL:    c.URL,
		Header: map[string][]string{
			"Range": {"npt=0.000-"},
		},
	}

	_, err = c.Do(req)
	return
}

func (c *Conn) SetupMedia(media *core.Media) (byte, error) {
	var transport string

	if c.Transport == "udp" {
		conn1, conn2, err := ListenUDPPair()
		if err != nil {
			return 0, err
		}

		c.udpConn = append(c.udpConn, conn1, conn2)

		port := conn1.LocalAddr().(*net.UDPAddr).Port
		transport = fmt.Sprintf("RTP/AVP;unicast;client_port=%d-%d", port, port+1)
	} else {
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
	}

	if transport == "" {
		return 0, fmt.Errorf("wrong media: %v", media)
	}

	rawURL := media.ID // control
	if !strings.Contains(rawURL, "://") {
		rawURL = c.URL.String()
		// prefix check for https://github.com/AlexxIT/go2rtc/issues/1236
		if !strings.HasSuffix(rawURL, "/") && !strings.HasPrefix(media.ID, "/") {
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
		// Session: 7116520596809429228
		// Session: 216525287999;timeout=60
		if s := res.Header.Get("Session"); s != "" {
			if i := strings.IndexByte(s, ';'); i > 0 {
				c.session = s[:i]
				if i = strings.Index(s, "timeout="); i > 0 {
					c.keepalive, _ = strconv.Atoi(s[i+8:])
				}
			} else {
				c.session = s
			}
		}
	}

	// Parse server response
	transport = res.Header.Get("Transport")

	if c.Transport == "udp" {
		channel := byte(len(c.udpConn) - 2)

		// Dahua:   RTP/AVP/UDP;unicast;client_port=49292-49293;server_port=43670-43671;ssrc=7CB694B4
		// OpenIPC: RTP/AVP/UDP;unicast;client_port=59612-59613
		if s := core.Between(transport, "server_port=", ";"); s != "" {
			s1, s2, _ := strings.Cut(s, "-")
			port1 := core.Atoi(s1)
			port2 := core.Atoi(s2)
			// TODO: more smart handling empty server ports
			if port1 > 0 && port2 > 0 {
				remoteIP := c.conn.RemoteAddr().(*net.TCPAddr).IP
				c.udpAddr = append(c.udpAddr,
					&net.UDPAddr{IP: remoteIP, Port: port1},
					&net.UDPAddr{IP: remoteIP, Port: port2},
				)

				go func() {
					// Try to open a hole in the NAT router (to allow incoming UDP packets)
					// by send a UDP packet for RTP and RTCP to the remote RTSP server.
					// https://github.com/FFmpeg/FFmpeg/blob/aa91ae25b88e195e6af4248e0ab30605735ca1cd/libavformat/rtpdec.c#L416-L438
					_, _ = c.WriteToUDP([]byte{0x80, 0x00, 0x00, 0x00}, channel)
					_, _ = c.WriteToUDP([]byte{0x80, 0xC8, 0x00, 0x01}, channel+1)
				}()
			}
		}

		return channel, nil
	} else {
		// we send our `interleaved`, but camera can answer with another

		// Transport: RTP/AVP/TCP;unicast;interleaved=10-11;ssrc=10117CB7
		// Transport: RTP/AVP/TCP;unicast;destination=192.168.1.111;source=192.168.1.222;interleaved=0
		// Transport: RTP/AVP/TCP;ssrc=22345682;interleaved=0-1
		// Escam Q6 has a bug:
		// Transport: RTP/AVP;unicast;destination=192.168.1.111;source=192.168.1.222;interleaved=0-1
		s := core.Between(transport, "interleaved=", "-")
		i, err := strconv.Atoi(s)
		if err != nil {
			return 0, fmt.Errorf("wrong transport: %s", transport)
		}

		return byte(i), nil
	}
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
	if c.OnClose != nil {
		_ = c.OnClose()
	}
	for _, conn := range c.udpConn {
		_ = conn.Close()
	}
	return c.conn.Close()
}

func (c *Conn) WriteToUDP(b []byte, channel byte) (int, error) {
	return c.udpConn[channel].WriteToUDP(b, c.udpAddr[channel])
}

const listenUDPAttemps = 10

var listenUDPMu sync.Mutex

func ListenUDPPair() (*net.UDPConn, *net.UDPConn, error) {
	listenUDPMu.Lock()
	defer listenUDPMu.Unlock()

	for i := 0; i < listenUDPAttemps; i++ {
		// Get a random even port from the OS
		ln1, err := net.ListenUDP("udp", &net.UDPAddr{IP: nil, Port: 0})
		if err != nil {
			continue
		}

		var port1 = ln1.LocalAddr().(*net.UDPAddr).Port
		var port2 int

		// 11. RTP over Network and Transport Protocols (https://www.ietf.org/rfc/rfc3550.txt)
		// For UDP and similar protocols,
		// RTP SHOULD use an even destination port number and the corresponding
		// RTCP stream SHOULD use the next higher (odd) destination port number
		if port1&1 > 0 {
			port2 = port1 - 1
		} else {
			port2 = port1 + 1
		}

		ln2, err := net.ListenUDP("udp", &net.UDPAddr{IP: nil, Port: port2})
		if err != nil {
			_ = ln1.Close()
			continue
		}

		if port1 < port2 {
			return ln1, ln2, nil
		} else {
			return ln2, ln1, nil
		}
	}

	return nil, nil, fmt.Errorf("can't open two UDP ports")
}
