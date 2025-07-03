package doorbird

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

var (
	clt Client
)

type Client struct {
	core.Connection
	conn net.Conn
}

func Dial(rawURL string) (*Client, error) {
	if clt.conn != nil {
		return &clt, nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	user := u.User.Username()
	pass, _ := u.User.Password()

	if u.Port() == "" {
		u.Host += ":80"
	}

	conn, err := net.DialTimeout("tcp", u.Host, core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}

	s := fmt.Sprintf("POST /bha-api/audio-transmit.cgi?http-user=%s&http-password=%s HTTP/1.0\r\n", user, pass) +
		"Content-Type: audio/basic\r\n" +
		"Content-Length: 9999999\r\n" +
		"Connection: Keep-Alive\r\n" +
		"Cache-Control: no-cache\r\n" +
		"\r\n"

	_ = conn.SetWriteDeadline(time.Now().Add(core.ConnDeadline))
	if _, err = conn.Write([]byte(s)); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(conn)
	statusLine, _ := reader.ReadString('\n')
	parts := strings.SplitN(statusLine, " ", 3)
	if len(parts) >= 2 {
		statusCode, err := strconv.Atoi(parts[1])
		if err == nil {
			if statusCode == 204 {
				conn.Close()
				return nil, fmt.Errorf("DoorBird user has no api permission: %d", statusCode)
			}
			if statusCode == 503 {
				conn.Close()
				return nil, fmt.Errorf("DoorBird device is busy: %d", statusCode)
			}
		}
	}

	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecPCMU, ClockRate: 8000},
			},
		},
	}

	clt = Client{
		core.Connection{
			ID:         core.NewID(),
			FormatName: "doorbird",
			Protocol:   "http",
			URL:        rawURL,
			Medias:     medias,
			// Transport:  conn,
		},
		conn,
	}

	return &clt, nil
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	if len(c.Senders) > 0 {
		return fmt.Errorf("DoorBird backchannel already in use")
	}

	sender := core.NewSender(media, track.Codec)

	sender.Handler = func(pkt *rtp.Packet) {
		if c.conn != nil {
			_ = c.conn.SetWriteDeadline(time.Now().Add(core.ConnDeadline))
			if n, err := c.conn.Write(pkt.Payload); err == nil {
				c.Send += n
			}
		}
	}

	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *Client) Start() error {
	if c.conn == nil {
		return nil
	}
	buf := make([]byte, 1)
	for {
		_, err := c.conn.Read(buf)
		if err != nil {
			c.conn.Close()
			c.conn = nil
			return err
		}
	}
}
