package rtsp

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
		uri:              uri,
		udpRtpConns:      make(map[byte]*UDPConnection),
		udpRtcpConns:     make(map[byte]*UDPConnection),
		udpRtpListeners:  make(map[byte]*UDPConnection),
		udpRtcpListeners: make(map[byte]*UDPConnection),
		portToChannel:    make(map[int]byte),
		channelCounter:   0,
	}
}

func (c *Conn) Dial() (err error) {
	if c.URL, err = url.Parse(c.uri); err != nil {
		return
	}

	var conn net.Conn

	if c.Transport == "" || c.Transport == "tcp" || c.Transport == "udp" {
		timeout := core.ConnDialTimeout
		if c.Timeout != 0 {
			timeout = time.Second * time.Duration(c.Timeout)
		}
		conn, err = tcp.Dial(c.URL, timeout)

		if c.Transport != "udp" {
			c.Protocol = "rtsp+tcp"
			c.transportMode = TransportTCP
		} else {
			c.Protocol = "rtsp+udp"
			c.transportMode = TransportUDP
		}
	} else {
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
	var mediaIndex int = -1

	// try to use media position as channel number
	for i, m := range c.Medias {
		if m.Equal(media) {
			mediaIndex = i
			break
		}
	}

	if mediaIndex == -1 {
		return 0, fmt.Errorf("wrong media: %v", media)
	}

	if c.transportMode == TransportUDP {
		transport, err := c.setupUDPTransport()
		if err == nil {
			return c.sendSetupRequest(media, transport)
		}
		// Fall back to TCP if UDP fails
		c.closeUDP()
		c.transportMode = TransportTCP
	}

	transport = c.setupTCPTransport(mediaIndex)
	return c.sendSetupRequest(media, transport)
}

func (c *Conn) setupTCPTransport(mediaIndex int) string {
	channel := byte(mediaIndex * 2)
	transport := fmt.Sprintf("RTP/AVP/TCP;unicast;interleaved=%d-%d", channel, channel+1)
	return transport
}

func (c *Conn) setupUDPTransport() (string, error) {
	portPair, err := GetUDPPorts(nil, 10)
	if err != nil {
		return "", err
	}

	rtpChannel := c.getChannelForPort(portPair.RTPPort)
	rtcpChannel := c.getChannelForPort(portPair.RTCPPort)

	c.udpRtpListeners[rtpChannel] = &UDPConnection{
		Conn:    *portPair.RTPListener,
		Channel: rtpChannel,
	}

	c.udpRtcpListeners[rtcpChannel] = &UDPConnection{
		Conn:    *portPair.RTCPListener,
		Channel: rtcpChannel,
	}

	transport := fmt.Sprintf("RTP/AVP;unicast;client_port=%d-%d", portPair.RTPPort, portPair.RTCPPort)
	return transport, nil
}

func (c *Conn) sendSetupRequest(media *core.Media, transport string) (byte, error) {
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
	responseTransport := res.Header.Get("Transport")

	if c.transportMode == TransportUDP {
		// Parse UDP response: client_ports=1234-1235;server_port=1234-1235
		var clientPorts []int
		var serverPorts []int

		if strings.Contains(transport, "client_port=") {
			parts := strings.Split(responseTransport, "client_port=")
			if len(parts) > 1 {
				portPart := strings.Split(strings.Split(parts[1], ";")[0], "-")
				for _, p := range portPart {
					if port, err := strconv.Atoi(p); err == nil {
						clientPorts = append(clientPorts, port)
					}
				}
			}
		}

		if strings.Contains(responseTransport, "server_port=") {
			parts := strings.Split(responseTransport, "server_port=")
			if len(parts) > 1 {
				portPart := strings.Split(strings.Split(parts[1], ";")[0], "-")
				for _, p := range portPart {
					if port, err := strconv.Atoi(p); err == nil {
						serverPorts = append(serverPorts, port)
					}
				}
			}
		}

		// Create UDP connections for RTP and RTCP if we have both server ports
		if len(serverPorts) >= 2 {
			if host, _, err := net.SplitHostPort(c.Connection.RemoteAddr); err == nil {
				rtpServerPort := serverPorts[0]
				rtcpServerPort := serverPorts[1]

				cleanHost := host
				if strings.Contains(cleanHost, ":") {
					cleanHost = fmt.Sprintf("[%s]", host)
				}

				remoteRtpAddr := fmt.Sprintf("%s:%d", cleanHost, rtpServerPort)
				remoteRtcpAddr := fmt.Sprintf("%s:%d", cleanHost, rtcpServerPort)

				if rtpAddr, err := net.ResolveUDPAddr("udp", remoteRtpAddr); err == nil {
					if rtpConn, err := net.DialUDP("udp", nil, rtpAddr); err == nil {
						channel := c.getChannelForPort(rtpServerPort)
						c.udpRtpConns[channel] = &UDPConnection{
							Conn:    *rtpConn,
							Channel: channel,
						}
					}
				}

				if rtcpAddr, err := net.ResolveUDPAddr("udp", remoteRtcpAddr); err == nil {
					if rtcpConn, err := net.DialUDP("udp", nil, rtcpAddr); err == nil {
						channel := c.getChannelForPort(rtcpServerPort)
						c.udpRtcpConns[channel] = &UDPConnection{
							Conn:    *rtcpConn,
							Channel: channel,
						}
					}
				}
			}
		}

		// Try to open a hole in the NAT router (to allow incoming UDP packets)
		// by send a UDP packet for RTP and RTCP to the remote RTSP server.
		go c.tryHolePunching(clientPorts, serverPorts)

		var rtpPort string
		if media.Direction == core.DirectionRecvonly {
			rtpPort = core.Between(transport, "client_port=", "-")
		} else {
			rtpPort = core.Between(responseTransport, "server_port=", "-")
		}

		i, err := strconv.Atoi(rtpPort)
		if err != nil {
			return 0, err
		}

		return c.getChannelForPort(i), nil

	} else {
		// we send our `interleaved`, but camera can answer with another

		// Transport: RTP/AVP/TCP;unicast;interleaved=10-11;ssrc=10117CB7
		// Transport: RTP/AVP/TCP;unicast;destination=192.168.1.111;source=192.168.1.222;interleaved=0
		// Transport: RTP/AVP/TCP;ssrc=22345682;interleaved=0-1
		if !strings.HasPrefix(responseTransport, "RTP/AVP/TCP;") {
			// Escam Q6 has a bug:
			// Transport: RTP/AVP;unicast;destination=192.168.1.111;source=192.168.1.222;interleaved=0-1
			if !strings.Contains(responseTransport, ";interleaved=") {
				return 0, fmt.Errorf("wrong transport: %s", responseTransport)
			}
		}

		channel := core.Between(responseTransport, "interleaved=", "-")
		i, err := strconv.Atoi(channel)
		if err != nil {
			return 0, err
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
	c.closeUDP()

	if c.mode == core.ModeActiveProducer {
		_ = c.Teardown()
	}

	if c.OnClose != nil {
		_ = c.OnClose()
	}

	return c.conn.Close()
}

func (c *Conn) closeUDP() {
	for _, listener := range c.udpRtpListeners {
		_ = listener.Conn.Close()
	}
	for _, listener := range c.udpRtcpListeners {
		_ = listener.Conn.Close()
	}
	for _, conn := range c.udpRtpConns {
		_ = conn.Conn.Close()
	}
	for _, conn := range c.udpRtcpConns {
		_ = conn.Conn.Close()
	}

	c.udpRtpListeners = make(map[byte]*UDPConnection)
	c.udpRtcpListeners = make(map[byte]*UDPConnection)
	c.udpRtpConns = make(map[byte]*UDPConnection)
	c.udpRtcpConns = make(map[byte]*UDPConnection)
	c.portToChannel = make(map[int]byte)
	c.channelCounter = 0
}

func (c *Conn) sendUDPRtpPacket(data []byte) error {
	for len(data) >= 4 && data[0] == '$' {
		channel := data[1]
		size := binary.BigEndian.Uint16(data[2:4])

		if len(data) < 4+int(size) {
			return fmt.Errorf("incomplete RTP packet: %d < %d", len(data), 4+size)
		}

		// Send RTP data without interleaved header
		rtpData := data[4 : 4+size]

		if conn, ok := c.udpRtpConns[channel]; ok {
			if err := conn.Conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
				return nil
			}

			if _, err := conn.Conn.Write(rtpData); err != nil {
				return err
			}
		}

		data = data[4+size:] // Move to next packet
	}

	return nil
}

func (c *Conn) tryHolePunching(clientPorts, serverPorts []int) {
	if len(clientPorts) < 2 || len(serverPorts) < 2 {
		return
	}

	host, _, _ := net.SplitHostPort(c.Connection.RemoteAddr)
	if strings.Contains(host, ":") {
		host = fmt.Sprintf("[%s]", host)
	}

	// RTP hole punch
	if rtpListener, ok := c.udpRtpListeners[c.getChannelForPort(clientPorts[0])]; ok {
		if addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, serverPorts[0])); err == nil {
			rtpListener.Conn.WriteToUDP([]byte{0x80, 0x00, 0x00, 0x00}, addr)
		}
	}

	// RTCP hole punch
	if rtcpListener, ok := c.udpRtcpListeners[c.getChannelForPort(clientPorts[1])]; ok {
		if addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, serverPorts[1])); err == nil {
			rtcpListener.Conn.WriteToUDP([]byte{0x80, 0xC8, 0x00, 0x01}, addr)
		}
	}
}

func (c *Conn) getChannelForPort(port int) byte {
	if channel, exists := c.portToChannel[port]; exists {
		return channel
	}

	c.channelCounter++
	if c.channelCounter == 0 {
		c.channelCounter = 1
	}

	channel := c.channelCounter
	c.portToChannel[port] = channel

	return channel
}
