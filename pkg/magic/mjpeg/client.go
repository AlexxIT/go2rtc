package mjpeg

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Client struct {
	rd *core.ReadSeeker

	media    *core.Media
	receiver *core.Receiver

	recv int
}

func NewClient(rd io.Reader) *Client {
	return &Client{
		rd: core.NewReadSeeker(rd),
		media: &core.Media{
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
	}
}

func (c *Client) GetMedias() []*core.Media {
	return []*core.Media{c.media}
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	if c.receiver == nil {
		c.receiver = core.NewReceiver(media, codec)
	}
	return c.receiver, nil
}

func (c *Client) Start() error {
	var buf []byte                     // total bufer
	b := make([]byte, core.BufferSize) // reading buffer

	for {
		// one JPEG end and next start
		i := bytes.Index(buf, []byte{0xFF, 0xD9, 0xFF, 0xD8})
		if i < 0 {
			n, err := c.rd.Read(b)
			if err != nil {
				return err
			}

			c.recv += n

			buf = append(buf, b[:n]...)

			// if we receive frame
			if n >= 2 && b[n-2] == 0xFF && b[n-1] == 0xD9 {
				i = len(buf)
			} else {
				continue
			}
		} else {
			i += 2
		}

		pkt := &rtp.Packet{
			Header:  rtp.Header{Timestamp: core.Now90000()},
			Payload: buf[:i],
		}
		c.receiver.WriteRTP(pkt)

		//log.Printf("[mjpeg] ts=%d size=%d", pkt.Header.Timestamp, len(pkt.Payload))

		buf = buf[i:]
	}
}

func (c *Client) Stop() error {
	if c.receiver != nil {
		c.receiver.Close()
	}
	if closer, ok := c.rd.Reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:      "MJPEG active producer",
		Medias:    []*core.Media{c.media},
		Receivers: []*core.Receiver{c.receiver},
		Recv:      c.recv,
	}
	return json.Marshal(info)
}
