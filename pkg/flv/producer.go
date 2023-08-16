package flv

import (
	"encoding/json"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (c *Client) GetMedias() []*core.Media {
	return c.medias
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	for _, track := range c.receivers {
		if track.Codec == codec {
			return track, nil
		}
	}
	track := core.NewReceiver(media, codec)
	if media.Kind == core.KindVideo {
		c.video = track
	} else {
		c.audio = track
	}
	c.receivers = append(c.receivers, track)
	return track, nil
}

func (c *Client) Start() error {
	return c.play()
}

func (c *Client) Stop() error {
	if closer, ok := c.rd.Reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:      "FLV active producer",
		URL:       c.URL,
		Medias:    c.medias,
		Receivers: c.receivers,
		Recv:      c.recv,
	}
	return json.Marshal(info)
}
