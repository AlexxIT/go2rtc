package mjpeg

import (
	"encoding/json"
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"strings"
	"sync/atomic"
)

func (c *Client) GetMedias() []*streamer.Media {
	if c.medias == nil {
		c.medias = []*streamer.Media{{
			Kind:      streamer.KindVideo,
			Direction: streamer.DirectionSendonly,
			Codecs: []*streamer.Codec{
				{
					Name: streamer.CodecJPEG, ClockRate: 90000, PayloadType: streamer.PayloadTypeRAW,
				},
			},
		}}
	}
	return c.medias
}

func (c *Client) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	if c.track == nil {
		c.track = streamer.NewTrack(codec, streamer.DirectionSendonly)
	}
	return c.track
}

func (c *Client) Start() error {
	ct := c.res.Header.Get("Content-Type")

	if ct == "image/jpeg" {
		return c.startJPEG()
	}

	// added in go1.18
	if _, s, ok := strings.Cut(ct, "boundary="); ok {
		return c.startMJPEG(s)
	}

	return errors.New("wrong Content-Type: " + ct)
}

func (c *Client) Stop() error {
	// important for close reader/writer gorutines
	_ = c.res.Body.Close()
	c.closed = true
	return nil
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &streamer.Info{
		Type:       "MJPEG source",
		URL:        c.res.Request.URL.String(),
		RemoteAddr: c.RemoteAddr,
		UserAgent:  c.UserAgent,
		Recv:       atomic.LoadUint32(&c.recv),
	}
	return json.Marshal(info)
}
