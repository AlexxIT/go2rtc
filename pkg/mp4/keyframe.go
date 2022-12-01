package mp4

import (
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
)

type Keyframe struct {
	streamer.Element

	MimeType string
}

func (c *Keyframe) GetMedias() []*streamer.Media {
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

func (c *Keyframe) AddTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	muxer := &Muxer{}

	codecs := []*streamer.Codec{track.Codec}

	init, err := muxer.GetInit(codecs)
	if err != nil {
		return nil
	}

	c.MimeType = muxer.MimeType(codecs)

	switch track.Codec.Name {
	case streamer.CodecH264:
		push := func(packet *rtp.Packet) error {
			if !h264.IsKeyframe(packet.Payload) {
				return nil
			}

			buf := muxer.Marshal(0, packet)
			c.Fire(append(init, buf...))

			return nil
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
