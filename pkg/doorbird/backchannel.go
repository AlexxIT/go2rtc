// backchannel audio stream for doorbird devices
// uses audio buffering and sends packets every second too meet doorbird api limitations
// as described on page 5: https://www.doorbird.com/downloads/api_lan.pdf?rev=0.36
package doorbird

import (
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Client struct {
	core.Connection
	conn net.Conn
}

var lastDialTime time.Time
var dialMutex sync.Mutex

const audioPacketInterval = 20 * time.Millisecond

func Dial(rawURL string) (*Client, error) {
	dialMutex.Lock()
	now := time.Now()
	wait := time.Duration(0)
	if !lastDialTime.IsZero() {
		elapsed := now.Sub(lastDialTime)
		if elapsed < time.Second {
			wait = time.Second - elapsed
		}
	}
	lastDialTime = now
	dialMutex.Unlock()
	if wait > 0 {
		time.Sleep(wait)
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

	// use a buffered channel for audio packets
	buffer := make(chan []byte, 256)

	sender.Handler = func(pkt *rtp.Packet) {
		select {
		case buffer <- pkt.Payload:
			// ok
		default:
			// buffer full, drop oldest
			<-buffer
			buffer <- pkt.Payload
		}
	}

	go func() {
		for payload := range buffer {
			_ = c.conn.SetWriteDeadline(time.Now().Add(core.ConnDeadline))
			if n, err := c.conn.Write(payload); err == nil {
				c.Send += n
			}
			time.Sleep(audioPacketInterval)
		}
	}()

	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *Client) Start() (err error) {
	_, err = c.conn.Read(nil)
	return
}
