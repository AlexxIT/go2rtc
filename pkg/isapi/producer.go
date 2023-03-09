package isapi

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
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

	track := streamer.NewTrack(codec, media.Direction)
	track = track.Bind(func(packet *rtp.Packet) (err error) {
		if c.conn != nil {
			c.send += len(packet.Payload)
			_, err = c.conn.Write(packet.Payload)
		}
		return
	})
	c.tracks = append(c.tracks, track)

	return track
}

func (c *Client) Start() (err error) {
	if err = c.Open(); err != nil {
		return
	}
	return
}

func (c *Client) Stop() (err error) {
	if c.conn == nil {
		return
	}
	_ = c.Close()
	return c.conn.Close()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &streamer.Info{
		Type:   "ISAPI",
		Medias: c.medias,
		Tracks: c.tracks,
		Send:   uint32(c.send),
	}
	return json.Marshal(info)
}
