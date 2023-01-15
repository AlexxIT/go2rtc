package rtmp

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
	return c.Handle()
}

func (c *Client) Stop() error {
	return c.Close()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &streamer.Info{
		Type:   "RTMP source",
		URL:    c.URI,
		Medias: c.medias,
		Tracks: c.tracks,
		Recv:   atomic.LoadUint32(&c.recv),
	}
	return json.Marshal(info)
}
