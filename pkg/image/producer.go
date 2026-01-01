package image

import (
	"errors"
	"io"
	"net/http"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection

	closed bool
	res    *http.Response
}

func Open(res *http.Response) (*Producer, error) {
	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "image",
			Protocol:   "http",
			RemoteAddr: res.Request.URL.Host,
			Transport:  res.Body,
			Medias: []*core.Media{
				{
					Kind:      core.KindVideo,
					Direction: core.DirectionRecvonly,
					Codecs: []*core.Codec{
						{
							Name:        core.CodecJPEG,
							ClockRate:   90000,
							PayloadType: core.PayloadTypeRAW,
						},
					},
				},
			},
		},
		res: res,
	}, nil
}

func (c *Producer) Start() error {
	body, err := io.ReadAll(c.res.Body)
	if err != nil {
		return err
	}

	pkt := &rtp.Packet{
		Header:  rtp.Header{Timestamp: core.Now90000()},
		Payload: body,
	}
	c.Receivers[0].WriteRTP(pkt)

	c.Recv += len(body)

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

		c.Recv += len(body)

		pkt = &rtp.Packet{
			Header:  rtp.Header{Timestamp: core.Now90000()},
			Payload: body,
		}
		c.Receivers[0].WriteRTP(pkt)
	}

	return nil
}

func (c *Producer) Stop() error {
	c.closed = true
	return c.Connection.Stop()
}
