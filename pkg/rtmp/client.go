package rtmp

import (
	"bufio"
	"io"
	"net"
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

func DialPlay(rawURL string) (*flv.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	conn, err := tcp.Dial(u, core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}

	client, err := NewClient(conn, u)
	if err != nil {
		return nil, err
	}

	if err = client.play(); err != nil {
		return nil, err
	}

	return client.Producer()
}

func DialPublish(rawURL string) (io.Writer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	conn, err := tcp.Dial(u, core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}

	client, err := NewClient(conn, u)
	if err != nil {
		return nil, err
	}

	if err = client.publish(); err != nil {
		return nil, err
	}

	return client, nil
}

func NewClient(conn net.Conn, u *url.URL) (*Conn, error) {
	c := &Conn{
		url: u.String(),

		conn: conn,
		rd:   bufio.NewReaderSize(conn, core.BufferSize),
		wr:   conn,

		chunks: map[uint8]*chunk{},

		rdPacketSize: 128,
		wrPacketSize: 4096, // OBS - 4096, Reolink - 4096
	}

	if args := strings.Split(u.Path, "/"); len(args) >= 2 {
		c.App = args[1]
		if len(args) >= 3 {
			c.Stream = args[2]
			if u.RawQuery != "" {
				c.Stream += "?" + u.RawQuery
			}
		}
	}

	if err := c.clienHandshake(); err != nil {
		return nil, err
	}
	if err := c.writePacketSize(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Conn) clienHandshake() error {
	// simple handshake without real random and check response
	b := make([]byte, 1+1536)
	b[0] = 0x03
	// write C0+C1
	if _, err := c.conn.Write(b); err != nil {
		return err
	}
	// read S0+S1
	if _, err := io.ReadFull(c.rd, b); err != nil {
		return err
	}
	// write S1
	if _, err := c.conn.Write(b[1:]); err != nil {
		return err
	}
	// read C1, skip check
	if _, err := io.ReadFull(c.rd, b[1:]); err != nil {
		return err
	}
	return nil
}

func (c *Conn) play() error {
	if err := c.writeConnect(); err != nil {
		return err
	}
	if err := c.writeCreateStream(); err != nil {
		return err
	}
	if err := c.writePlay(); err != nil {
		return err
	}
	return nil
}

func (c *Conn) publish() error {
	if err := c.writeConnect(); err != nil {
		return err
	}
	if err := c.writeReleaseStream(); err != nil {
		return err
	}
	if err := c.writeCreateStream(); err != nil {
		return err
	}
	if err := c.writePublish(); err != nil {
		return err
	}

	go func() {
		for {
			_, _, _, err := c.readMessage()
			//log.Printf("!!! %d %d %.30x", msgType, timeMS, b)
			if err != nil {
				return
			}
		}
	}()

	return nil
}
