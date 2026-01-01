package aac

import (
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Consumer struct {
	core.Connection
	wr *core.WriteBuffer
}

func NewConsumer() *Consumer {
	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecAAC},
			},
		},
	}
	wr := core.NewWriteBuffer(nil)
	return &Consumer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "adts",
			Medias:     medias,
			Transport:  wr,
		},
		wr: wr,
	}
}

func (c *Consumer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	sender := core.NewSender(media, track.Codec)

	sender.Handler = func(pkt *rtp.Packet) {
		if n, err := c.wr.Write(pkt.Payload); err == nil {
			c.Send += n
		}
	}

	if track.Codec.IsRTP() {
		sender.Handler = RTPToADTS(track.Codec, sender.Handler)
	} else {
		sender.Handler = EncodeToADTS(track.Codec, sender.Handler)
	}

	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *Consumer) WriteTo(wr io.Writer) (int64, error) {
	return c.wr.WriteTo(wr)
}
