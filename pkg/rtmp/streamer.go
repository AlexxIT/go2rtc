package rtmp

import (
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"strconv"
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
	v := map[string]interface{}{
		streamer.JSONReceive:    c.receive,
		streamer.JSONType:       "RTMP client producer",
		streamer.JSONRemoteAddr: c.conn.NetConn().RemoteAddr().String(),
		"url":                   c.URI,
	}
	for i, media := range c.medias {
		k := "media:" + strconv.Itoa(i)
		v[k] = media.String()
	}
	for i, track := range c.tracks {
		k := "track:" + strconv.Itoa(i)
		v[k] = track.String()
	}
	return json.Marshal(v)
}
