package stdin

import (
	"encoding/json"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func (c *Client) GetMedias() []*core.Media {
	return c.medias
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (c *Client) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	if c.sender == nil {
		c.sender = core.NewSender(media, track.Codec)
		c.sender.Handler = func(packet *rtp.Packet) {
			_, _ = c.pipe.Write(packet.Payload)
			c.send += len(packet.Payload)
		}
	}

	c.sender.HandleRTP(track)
	return nil
}

func (c *Client) Start() (err error) {
	return c.cmd.Run()
}

func (c *Client) Stop() (err error) {
	if c.sender != nil {
		c.sender.Close()
	}
	return c.pipe.Close()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:   "Exec active consumer",
		Medias: c.medias,
		Send:   c.send,
	}
	if c.sender != nil {
		info.Senders = []*core.Sender{c.sender}
	}
	return json.Marshal(info)
}
