package mjpeg

import (
	"encoding/json"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Consumer struct {
	UserAgent  string
	RemoteAddr string

	medias []*core.Media
	sender *core.Sender

	wr *core.WriteBuffer

	send int
}

func (c *Consumer) GetMedias() []*core.Media {
	if c.medias == nil {
		c.medias = []*core.Media{
			{
				Kind:      core.KindVideo,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{Name: core.CodecJPEG},
				},
			},
		}
	}
	return c.medias
}

func (c *Consumer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	if c.wr == nil {
		c.wr = core.NewWriteBuffer(nil)
	}

	if c.sender == nil {
		c.sender = core.NewSender(media, track.Codec)
		c.sender.Handler = func(packet *rtp.Packet) {
			if n, err := c.wr.Write(packet.Payload); err == nil {
				c.send += n
			}
		}

		if track.Codec.IsRTP() {
			c.sender.Handler = RTPDepay(c.sender.Handler)
		}
	}

	c.sender.HandleRTP(track)
	return nil
}

func (c *Consumer) WriteTo(wr io.Writer) (int64, error) {
	return c.wr.WriteTo(wr)
}

func (c *Consumer) Stop() error {
	if c.sender != nil {
		c.sender.Close()
	}
	if c.wr != nil {
		_ = c.wr.Close()
	}
	return nil
}

func (c *Consumer) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:       "MJPEG passive consumer",
		RemoteAddr: c.RemoteAddr,
		UserAgent:  c.UserAgent,
		Medias:     c.medias,
		Send:       c.send,
	}
	if c.sender != nil {
		info.Senders = []*core.Sender{c.sender}
	}
	return json.Marshal(info)
}
