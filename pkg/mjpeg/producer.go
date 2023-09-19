package mjpeg

import (
	"encoding/json"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (c *Client) GetMedias() []*core.Media {
	if c.medias == nil {
		c.medias = []*core.Media{{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{
					Name:        core.CodecJPEG,
					ClockRate:   90000,
					PayloadType: core.PayloadTypeRAW,
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
	// https://github.com/AlexxIT/go2rtc/issues/278
	return c.Handle()
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
		Type:       "JPEG active producer",
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
