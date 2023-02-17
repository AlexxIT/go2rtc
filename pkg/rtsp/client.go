package rtsp

import (
	"bufio"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
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
	"sync"
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
	ModeUnknown        Mode = iota
	ModeClientProducer      // conn act as RTSP client that receive data from RTSP server (ex. camera)
	ModeServerUnknown
	ModeServerProducer // conn act as RTSP server that reseive data from RTSP client (ex. ffmpeg output)
	ModeServerConsumer // conn act as RTSP server that send data to RTSP client (ex. ffmpeg input)
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

type Conn struct {
	streamer.Element

	// public

	Backchannel bool
	SessionName string

	Medias    []*streamer.Media
	Session   string
	UserAgent string
	URL       *url.URL

	// internal

	auth     *tcp.Auth
	conn     net.Conn
	mode     Mode
	state    State
	stateMu  sync.Mutex
	reader   *bufio.Reader
	sequence int
	uri      string

	tracks   []*streamer.Track
	channels map[byte]*streamer.Track

	// stats

	receive int
	send    int
}

func NewClient(uri string) (*Conn, error) {
	c := new(Conn)
	c.mode = ModeClientProducer
	c.uri = uri
	return c, c.parseURI()
}

func (c *Conn) parseURI() (err error) {
	c.URL, err = url.Parse(c.uri)
	if err != nil {
		return err
	}

	if strings.IndexByte(c.URL.Host, ':') < 0 {
		c.URL.Host += ":554"
	}

	// remove UserInfo from URL
	c.auth = tcp.NewAuth(c.URL.User)
	c.URL.User = nil

	return nil
}

func (c *Conn) Dial() (err error) {
	if c.conn != nil {
		_ = c.parseURI()
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

	c.mode = ModeClientProducer

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

	req.Body, err = streamer.MarshalSDP(c.SessionName, c.Medias)
	if err != nil {
		return err
	}

	res, err := c.Do(req)

	_ = res

	return
}

func (c *Conn) Setup() error {
	for _, media := range c.Medias {
		_, err := c.SetupMedia(media, media.Codecs[0], true)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Conn) SetupMedia(media *streamer.Media, codec *streamer.Codec, first bool) (*streamer.Track, error) {
	// TODO: rewrite recoonection and first flag
	if first {
		c.stateMu.Lock()
		defer c.stateMu.Unlock()
	}

	if c.state != StateConn && c.state != StateSetup {
		return nil, fmt.Errorf("RTSP SETUP from wrong state: %s", c.state)
	}

	ch := c.GetChannel(media)
	if ch < 0 {
		return nil, fmt.Errorf("wrong media: %v", media)
	}

	rawURL := media.Control
	if !strings.Contains(rawURL, "://") {
		rawURL = c.URL.String()
		if !strings.HasSuffix(rawURL, "/") {
			rawURL += "/"
		}
		rawURL += media.Control
	}
	trackURL, err := urlParse(rawURL)
	if err != nil {
		return nil, err
	}

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
		// some Dahua/Amcrest cameras fail here because two simultaneous
		// backchannel connections
		if c.Backchannel {
			c.Backchannel = false
			if err := c.Dial(); err != nil {
				return nil, err
			}
			if err := c.Describe(); err != nil {
				return nil, err
			}

			for _, newMedia := range c.Medias {
				if newMedia.Control == media.Control {
					return c.SetupMedia(newMedia, newMedia.Codecs[0], false)
				}
			}
		}

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

	// in case the track has already been setup before
	if codec == nil {
		c.state = StateSetup
		return nil, nil
	}

	// we send our `interleaved`, but camera can answer with another

	// Transport: RTP/AVP/TCP;unicast;interleaved=10-11;ssrc=10117CB7
	// Transport: RTP/AVP/TCP;unicast;destination=192.168.1.111;source=192.168.1.222;interleaved=0
	// Transport: RTP/AVP/TCP;ssrc=22345682;interleaved=0-1
	s := res.Header.Get("Transport")
	// TODO: rewrite
	if !strings.HasPrefix(s, "RTP/AVP/TCP;") {
		// Escam Q6 has a bug:
		// Transport: RTP/AVP;unicast;destination=192.168.1.111;source=192.168.1.222;interleaved=0-1
		if !strings.Contains(s, ";interleaved=") {
			return nil, fmt.Errorf("wrong transport: %s", s)
		}
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

	track := streamer.NewTrack(codec, media.Direction)

	switch track.Direction {
	case streamer.DirectionSendonly:
		if c.channels == nil {
			c.channels = make(map[byte]*streamer.Track)
		}
		c.channels[byte(ch)] = track

	case streamer.DirectionRecvonly:
		track = c.bindTrack(track, byte(ch), codec.PayloadType)
	}

	c.state = StateSetup
	c.tracks = append(c.tracks, track)

	return track, nil
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

	defer func() {
		c.stateMu.Lock()
		defer c.stateMu.Unlock()

		if c.state == StateNone {
			err = nil
			return
		}

		// may have gotten here because of the deadline
		// so close the connection to stop keepalive
		c.state = StateNone
		_ = c.conn.Close()
	}()

	var timeout time.Duration

	switch c.mode {
	case ModeClientProducer:
		// polling frames from remote RTSP Server (ex Camera)
		go c.keepalive()

		if c.HasSendTracks() {
			// if we receiving video/audio from camera
			timeout = time.Second * 5
		} else {
			// if we only send audio to camera
			timeout = time.Second * 30
		}

	case ModeServerProducer:
		// polling frames from remote RTSP Client (ex FFmpeg)
		timeout = time.Second * 15

	case ModeServerConsumer:
		// pushing frames to remote RTSP Client (ex VLC)
		timeout = time.Second * 60

	default:
		return fmt.Errorf("wrong RTSP conn mode: %d", c.mode)
	}

	for {
		if c.state == StateNone {
			return
		}

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

					// check if channel ID exists
					if c.channels[channelID] == nil {
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

		c.receive += int(size)

		if channelID&1 == 0 {
			packet := &rtp.Packet{}
			if err = packet.Unmarshal(buf); err != nil {
				return
			}

			track := c.channels[channelID]
			if track != nil {
				_ = track.WriteRTP(packet)
			} else {
				//c.Fire("wrong channelID: " + strconv.Itoa(int(channelID)))
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

func (c *Conn) GetChannel(media *streamer.Media) int {
	for i, m := range c.Medias {
		if m == media {
			return i
		}
	}
	return -1
}

func (c *Conn) HasSendTracks() bool {
	for _, track := range c.tracks {
		if track.Direction == streamer.DirectionSendonly {
			return true
		}
	}
	return false
}
