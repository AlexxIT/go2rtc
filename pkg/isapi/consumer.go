package isapi

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
			if c.conn == nil {
				return
			}
			c.send += len(packet.Payload)
			_, _ = c.conn.Write(packet.Payload)
		}
	}

	c.sender.HandleRTP(track)
	return nil
}

func (c *Client) Start() (err error) {
	if err = c.Open(); err != nil {
		return
	}
	return
}

func (c *Client) Stop() (err error) {
	if c.sender != nil {
		c.sender.Close()
	}

	if c.conn != nil {
		_ = c.Close()
		return c.conn.Close()
	}

	return nil
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:   "ISAPI active consumer",
		Medias: c.medias,
		Send:   c.send,
	}
	if c.sender != nil {
		info.Senders = []*core.Sender{c.sender}
	}
	return json.Marshal(info)
}
