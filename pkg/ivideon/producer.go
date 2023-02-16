package ivideon

import (
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"sync/atomic"
)

func (c *Client) GetMedias() []*streamer.Media {
	return c.medias
}

func (c *Client) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	for _, track := range c.tracks {
		if track.Codec == codec {
			return track
		}
	}
	panic(fmt.Sprintf("wrong media/codec: %+v %+v", media, codec))
}

func (c *Client) Start() error {
	err := c.Handle()
	if c.buffer == nil {
		return nil
	}
	return err
}

func (c *Client) Stop() error {
	return c.Close()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	var tracks []*streamer.Track
	for _, track := range c.tracks {
		tracks = append(tracks, track)
	}

	info := &streamer.Info{
		Type:   "Ivideon source",
		URL:    c.ID,
		Medias: c.medias,
		Tracks: tracks,
		Recv:   atomic.LoadUint32(&c.recv),
	}
	return json.Marshal(info)
}
