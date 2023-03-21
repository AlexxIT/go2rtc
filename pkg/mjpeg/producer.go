package mjpeg

import (
	"encoding/json"
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"strings"
)

func (c *Client) GetMedias() []*core.Media {
	if c.medias == nil {
		c.medias = []*core.Media{{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{
					Name: core.CodecJPEG, ClockRate: 90000, PayloadType: core.PayloadTypeRAW,
				},
			},
		}}
	}
	return c.medias
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	if c.receiver == nil {
		c.receiver = core.NewReceiver(media, codec)
	}
	return c.receiver, nil
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
	if c.receiver != nil {
		c.receiver.Close()
	}
	// important for close reader/writer gorutines
	_ = c.res.Body.Close()
	c.closed = true
	return nil
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:       "MJPEG active producer",
		URL:        c.res.Request.URL.String(),
		RemoteAddr: c.RemoteAddr,
		UserAgent:  c.UserAgent,
		Medias:     c.medias,
		Recv:       c.recv,
	}
	if c.receiver != nil {
		info.Receivers = []*core.Receiver{c.receiver}
	}
	return json.Marshal(info)
}
