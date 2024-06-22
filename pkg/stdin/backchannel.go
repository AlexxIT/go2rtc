package stdin

import (
	"encoding/json"
	"errors"

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
		stdin, err := c.cmd.StdinPipe()
		if err != nil {
			return err
		}

		c.sender = core.NewSender(media, track.Codec)
		c.sender.Handler = func(packet *rtp.Packet) {
			_, _ = stdin.Write(packet.Payload)
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
	if c.cmd.Process == nil {
		return nil
	}
	return errors.Join(c.cmd.Process.Kill(), c.cmd.Wait())
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Connection{
		ID:         core.ID(c),
		FormatName: "exec",
		Protocol:   "pipe",
		Medias:     c.medias,
		Send:       c.send,
	}
	if c.sender != nil {
		info.Senders = []*core.Sender{c.sender}
	}
	return json.Marshal(info)
}
