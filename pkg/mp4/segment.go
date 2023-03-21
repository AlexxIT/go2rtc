package mp4

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

type Segment struct {
	core.Listener

	Medias     []*core.Media
	UserAgent  string
	RemoteAddr string

	senders []*core.Sender

	MimeType     string
	OnlyKeyframe bool

	send int
}

func (c *Segment) GetMedias() []*core.Media {
	if c.Medias != nil {
		return c.Medias
	}

	// default local medias
	return []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecH264},
				{Name: core.CodecH265},
			},
		},
	}
}

func (c *Segment) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	muxer := &Muxer{}

	codecs := []*core.Codec{track.Codec}

	init, err := muxer.GetInit(codecs)
	if err != nil {
		return nil
	}

	c.MimeType = `video/mp4; codecs="` + muxer.MimeCodecs(codecs) + `"`

	handler := core.NewSender(media, track.Codec)

	switch track.Codec.Name {
	case core.CodecH264:

		if c.OnlyKeyframe {
			handler.Handler = func(packet *rtp.Packet) {
				if !h264.IsKeyframe(packet.Payload) {
					return
				}

				buf := muxer.Marshal(0, packet)
				c.Fire(append(init, buf...))

				c.send += len(buf)
			}
		} else {
			var buf []byte

			handler.Handler = func(packet *rtp.Packet) {
				if h264.IsKeyframe(packet.Payload) {
					// fist frame - send only IFrame
					// other frames - send IFrame and all PFrames
					if buf == nil {
						buf = append(buf, init...)
						b := muxer.Marshal(0, packet)
						buf = append(buf, b...)
					}

					c.Fire(buf)

					c.send += len(buf)

					buf = buf[:0]
					buf = append(buf, init...)
					muxer.Reset()
				}

				if buf != nil {
					b := muxer.Marshal(0, packet)
					buf = append(buf, b...)
				}
			}
		}

		if track.Codec.IsRTP() {
			handler.Handler = h264.RTPDepay(track.Codec, handler.Handler)
		} else {
			handler.Handler = h264.RepairAVC(track.Codec, handler.Handler)
		}

	case core.CodecH265:
		handler.Handler = func(packet *rtp.Packet) {
			if !h265.IsKeyframe(packet.Payload) {
				return
			}

			buf := muxer.Marshal(0, packet)
			c.Fire(append(init, buf...))

			c.send += len(buf)
		}

		if track.Codec.IsRTP() {
			handler.Handler = h265.RTPDepay(track.Codec, handler.Handler)
		}

	default:
		panic(core.UnsupportedCodec)
	}

	handler.HandleRTP(track)
	c.senders = append(c.senders, handler)

	return nil
}

func (c *Segment) Stop() error {
	for _, sender := range c.senders {
		sender.Close()
	}
	return nil
}

func (c *Segment) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:       "MP4/WebSocket passive consumer",
		RemoteAddr: c.RemoteAddr,
		UserAgent:  c.UserAgent,
		Medias:     c.Medias,
		Senders:    c.senders,
		Send:       c.send,
	}
	return json.Marshal(info)
}
