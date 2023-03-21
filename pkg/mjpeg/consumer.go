package mjpeg

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Consumer struct {
	core.Listener

	UserAgent  string
	RemoteAddr string

	medias []*core.Media
	sender *core.Sender

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
	if c.sender == nil {
		c.sender = core.NewSender(media, track.Codec)
		c.sender.Handler = func(packet *rtp.Packet) {
			c.Fire(packet.Payload)
			c.send += len(packet.Payload)
		}

		if track.Codec.IsRTP() {
			c.sender.Handler = RTPDepay(c.sender.Handler)
		}
	}

	c.sender.HandleRTP(track)
	return nil
}

func (c *Consumer) Stop() error {
	if c.sender != nil {
		c.sender.Close()
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
