package mjpeg

import (
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Consumer struct {
	core.SuperConsumer
	wr *core.WriteBuffer
}

func NewConsumer() *Consumer {
	return &Consumer{
		core.SuperConsumer{
			Type: "MJPEG passive consumer",
			Medias: []*core.Media{
				{
					Kind:      core.KindVideo,
					Direction: core.DirectionSendonly,
					Codecs: []*core.Codec{
						{Name: core.CodecJPEG},
					},
				},
			},
		},
		core.NewWriteBuffer(nil),
	}
}

func (c *Consumer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		if n, err := c.wr.Write(packet.Payload); err == nil {
			c.Send += n
		}
	}

	if track.Codec.IsRTP() {
		sender.Handler = RTPDepay(sender.Handler)
	}

	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *Consumer) WriteTo(wr io.Writer) (int64, error) {
	return c.wr.WriteTo(wr)
}

func (c *Consumer) Stop() error {
	_ = c.SuperConsumer.Close()
	return c.wr.Close()
}
