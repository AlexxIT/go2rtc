package hls

import (
	"encoding/json"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
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
	switch codec.Name {
	case core.CodecH264:
		track.ID = mpegts.StreamTypeH264
	case core.CodecAAC:
		track.ID = mpegts.StreamTypeAAC
	}

	c.receivers = append(c.receivers, track)
	return track, nil
}

func (c *Client) Start() error {
	return c.Handle()
}

func (c *Client) Stop() error {
	for _, receiver := range c.receivers {
		receiver.Close()
	}
	return nil
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:      "HLS active producer",
		URL:       c.playlist,
		Medias:    c.medias,
		Receivers: c.receivers,
		Recv:      c.recv,
	}
	return json.Marshal(info)
}
