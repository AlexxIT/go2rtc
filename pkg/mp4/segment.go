package mp4

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"sync/atomic"
)

type Segment struct {
	streamer.Element

	Medias     []*streamer.Media
	UserAgent  string
	RemoteAddr string

	MimeType     string
	OnlyKeyframe bool

	send uint32
}

func (c *Segment) GetMedias() []*streamer.Media {
	if c.Medias != nil {
		return c.Medias
	}

	// default medias
	return []*streamer.Media{
		{
			Kind:      streamer.KindVideo,
			Direction: streamer.DirectionRecvonly,
			Codecs: []*streamer.Codec{
				{Name: streamer.CodecH264},
				{Name: streamer.CodecH265},
			},
		},
	}
}

func (c *Segment) AddTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	muxer := &Muxer{}

	codecs := []*streamer.Codec{track.Codec}

	init, err := muxer.GetInit(codecs)
	if err != nil {
		return nil
	}

	c.MimeType = `video/mp4; codecs="` + muxer.MimeCodecs(codecs) + `"`

	switch track.Codec.Name {
	case streamer.CodecH264:
		var push streamer.WriterFunc

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

		var wrapper streamer.WrapperFunc
		if track.Codec.IsRTP() {
			wrapper = h264.RTPDepay(track)
		} else {
			wrapper = h264.RepairAVC(track)
		}
		push = wrapper(push)

		return track.Bind(push)

	case streamer.CodecH265:
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
	info := &streamer.Info{
		Type:       "WS/MP4 client",
		RemoteAddr: c.RemoteAddr,
		UserAgent:  c.UserAgent,
		Send:       atomic.LoadUint32(&c.send),
	}
	return json.Marshal(info)
}
