package mp4

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
	"sync/atomic"
)

type Segment struct {
	core.Listener

	Medias     []*core.Media
	UserAgent  string
	RemoteAddr string

	MimeType     string
	OnlyKeyframe bool

	send uint32
}

func (c *Segment) GetMedias() []*core.Media {
	if c.Medias != nil {
		return c.Medias
	}

	// default medias
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

func (c *Segment) AddTrack(media *core.Media, track *core.Track) *core.Track {
	muxer := &Muxer{}

	codecs := []*core.Codec{track.Codec}

	init, err := muxer.GetInit(codecs)
	if err != nil {
		return nil
	}

	c.MimeType = muxer.MimeType(codecs)

	switch track.Codec.Name {
	case core.CodecH264:
		var push core.WriterFunc

		if c.OnlyKeyframe {
			push = func(packet *rtp.Packet) error {
				if !h264.IsKeyframe(packet.Payload) {
					return nil
				}

				buf := muxer.Marshal(0, packet)
				atomic.AddUint32(&c.send, uint32(len(buf)))
				c.Fire(append(init, buf...))

				return nil
			}
		} else {
			var buf []byte

			push = func(packet *rtp.Packet) error {
				if h264.IsKeyframe(packet.Payload) {
					// fist frame - send only IFrame
					// other frames - send IFrame and all PFrames
					if buf == nil {
						buf = append(buf, init...)
						b := muxer.Marshal(0, packet)
						buf = append(buf, b...)
					}

					atomic.AddUint32(&c.send, uint32(len(buf)))
					c.Fire(buf)

					buf = buf[:0]
					buf = append(buf, init...)
					muxer.Reset()
				}

				if buf != nil {
					b := muxer.Marshal(0, packet)
					buf = append(buf, b...)
				}

				return nil
			}
		}

		var wrapper core.WrapperFunc
		if track.Codec.IsRTP() {
			wrapper = h264.RTPDepay(track)
		} else {
			wrapper = h264.RepairAVC(track)
		}
		push = wrapper(push)

		return track.Bind(push)

	case core.CodecH265:
		push := func(packet *rtp.Packet) error {
			if !h265.IsKeyframe(packet.Payload) {
				return nil
			}

			buf := muxer.Marshal(0, packet)
			atomic.AddUint32(&c.send, uint32(len(buf)))
			c.Fire(append(init, buf...))

			return nil
		}

		if track.Codec.IsRTP() {
			wrapper := h265.RTPDepay(track)
			push = wrapper(push)
		}

		return track.Bind(push)
	}

	panic("unsupported codec")
}

func (c *Segment) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:       "WS/MP4 client",
		RemoteAddr: c.RemoteAddr,
		UserAgent:  c.UserAgent,
		Send:       atomic.LoadUint32(&c.send),
	}
	return json.Marshal(info)
}
