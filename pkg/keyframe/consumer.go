package keyframe

import (
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
)

var annexB = []byte{0, 0, 0, 1}

type Consumer struct {
	streamer.Element
	IsMP4 bool
}

func (k *Consumer) GetMedias() []*streamer.Media {
	// support keyframe extraction only for one coded...
	codec := streamer.NewCodec(streamer.CodecH264)
	return []*streamer.Media{
		{
			Kind: streamer.KindVideo, Direction: streamer.DirectionRecvonly,
			Codecs: []*streamer.Codec{codec},
		},
	}
}

func (k *Consumer) AddTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	// sps and pps without AVC headers
	sps, pps := h264.GetParameterSet(track.Codec.FmtpLine)

	push := func(packet *rtp.Packet) error {
		// TODO: remove it, unnecessary
		if packet.Version != h264.RTPPacketVersionAVC {
			panic("wrong packet type")
		}

		switch h264.NALUType(packet.Payload) {
		case h264.NALUTypeSPS:
			sps = packet.Payload[4:] // remove AVC header
		case h264.NALUTypePPS:
			pps = packet.Payload[4:] // remove AVC header
		case h264.NALUTypeIFrame:
			if sps == nil || pps == nil {
				return nil
			}

			var data []byte

			if k.IsMP4 {
				data = mp4.MarshalMP4(sps, pps, packet.Payload)
			} else {
				data = append(data, annexB...)
				data = append(data, sps...)
				data = append(data, annexB...)
				data = append(data, pps...)
				data = append(data, annexB...)
				data = append(data, packet.Payload[4:]...)
			}

			k.Fire(data)
		}
		return nil
	}

	if !h264.IsAVC(track.Codec) {
		wrapper := h264.RTPDepay(track)
		push = wrapper(push)
	}

	return track.Bind(push)
}
