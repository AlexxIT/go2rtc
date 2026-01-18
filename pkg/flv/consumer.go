package flv

import (
	"io"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
)

type Consumer struct {
	core.Connection
	wr    *core.WriteBuffer
	muxer *Muxer
}

func NewConsumer() *Consumer {
	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecH264},
			},
		},
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
			FormatName: "flv",
			Medias:     medias,
			Transport:  wr,
		},
		wr:    wr,
		muxer: &Muxer{},
	}
}

func (c *Consumer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	sender := core.NewSender(media, track.Codec)

	switch track.Codec.Name {
	case core.CodecH264:
		payload := c.muxer.GetPayloader(track.Codec)

		sender.Handler = func(pkt *rtp.Packet) {
			b := payload(pkt)
			if n, err := c.wr.Write(b); err == nil {
				c.Send += n
			}
		}

		if track.Codec.IsRTP() {
			sender.Handler = h264.RTPDepay(track.Codec, sender.Handler)
		} else {
			sender.Handler = h264.RepairAVCC(track.Codec, sender.Handler)
		}

	case core.CodecAAC:
		payload := c.muxer.GetPayloader(track.Codec)

		sender.Handler = func(pkt *rtp.Packet) {
			b := payload(pkt)
			if n, err := c.wr.Write(b); err == nil {
				c.Send += n
			}
		}

		if track.Codec.IsRTP() {
			sender.Handler = aac.RTPDepay(sender.Handler)
		}
	}

	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *Consumer) WriteTo(wr io.Writer) (int64, error) {
	b := c.muxer.GetInit()
	if _, err := wr.Write(b); err != nil {
		return 0, err
	}
	return c.wr.WriteTo(wr)
}
