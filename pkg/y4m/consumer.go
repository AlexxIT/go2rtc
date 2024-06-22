package y4m

import (
	"fmt"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Consumer struct {
	core.Connection
	wr *core.WriteBuffer
}

func NewConsumer() *Consumer {
	wr := core.NewWriteBuffer(nil)
	return &Consumer{
		core.Connection{
			ID:         core.NewID(),
			Transport:  wr,
			FormatName: "yuv4mpegpipe",
			Medias: []*core.Media{
				{
					Kind:      core.KindVideo,
					Direction: core.DirectionSendonly,
					Codecs: []*core.Codec{
						{Name: core.CodecRAW},
					},
				},
			},
		},
		wr,
	}
}

func (c *Consumer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		if n, err := c.wr.Write([]byte(frameHdr)); err == nil {
			c.Send += n
		}
		if n, err := c.wr.Write(packet.Payload); err == nil {
			c.Send += n
		}
	}

	hdr := fmt.Sprintf(
		"YUV4MPEG2 W%s H%s C%s\n",
		core.Between(track.Codec.FmtpLine, "width=", ";"),
		core.Between(track.Codec.FmtpLine, "height=", ";"),
		core.Between(track.Codec.FmtpLine, "colorspace=", ";"),
	)
	if _, err := c.wr.Write([]byte(hdr)); err != nil {
		return err
	}

	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *Consumer) WriteTo(wr io.Writer) (int64, error) {
	return c.wr.WriteTo(wr)
}
