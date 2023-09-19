package mjpeg

import (
	"errors"
	"io"
	"net/http"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
)

type Client struct {
	core.Listener

	UserAgent  string
	RemoteAddr string

	closed bool
	res    *http.Response

	medias   []*core.Media
	receiver *core.Receiver

	recv int
}

func NewClient(res *http.Response) *Client {
	return &Client{res: res}
}

func (c *Client) Handle() error {
	body, err := io.ReadAll(c.res.Body)
	if err != nil {
		return err
	}

	pkt := &rtp.Packet{
		Header:  rtp.Header{Timestamp: core.Now90000()},
		Payload: body,
	}
	c.receiver.WriteRTP(pkt)

	c.recv += len(body)

	req := c.res.Request

	for !c.closed {
		res, err := tcp.Do(req)
		if err != nil {
			return err
		}

		if res.StatusCode != http.StatusOK {
			return errors.New("wrong status: " + res.Status)
		}

		body, err = io.ReadAll(res.Body)
		if err != nil {
			return err
		}

		c.recv += len(body)

		if c.receiver != nil {
			pkt = &rtp.Packet{
				Header:  rtp.Header{Timestamp: core.Now90000()},
				Payload: body,
			}
			c.receiver.WriteRTP(pkt)
		}
	}

	return nil
}
