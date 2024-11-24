package doorbird

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Client struct {
	core.Connection
	conn net.Conn
}

func Dial(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	user := u.User.Username()
	pass, _ := u.User.Password()

	rawURL = fmt.Sprintf("http://%s/bha-api/audio-transmit.cgi?http-user=%s&&http-password=%s", u.Host, user, pass)

	req, err := http.NewRequest("POST", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header = http.Header{
		"Content-Type":   []string{"audio/basic"},
		"Content-Length": []string{"9999999"},
		"Connection":     []string{"Keep-Alive"},
		"Cache-Control":  []string{"no-cache"},
	}

	if u.Port() == "" {
		u.Host += ":80"
	}

	conn, err := net.DialTimeout("tcp", u.Host, core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}

	_ = conn.SetWriteDeadline(time.Now().Add(core.ConnDeadline))
	if err = req.Write(conn); err != nil {
		return nil, err
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

	return &Client{
		core.Connection{
			ID:         core.NewID(),
			FormatName: "doorbird",
			Protocol:   "http",
			URL:        rawURL,
			Medias:     medias,
			Transport:  conn,
		},
		conn,
	}, nil
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	sender := core.NewSender(media, track.Codec)

	sender.Handler = func(pkt *rtp.Packet) {
		_ = c.conn.SetWriteDeadline(time.Now().Add(core.ConnDeadline))
		if n, err := c.conn.Write(pkt.Payload); err == nil {
			c.Send += n
		}
	}

	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *Client) Start() (err error) {
	_, err = c.conn.Read(nil)
	return
}
