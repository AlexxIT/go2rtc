package magic

import (
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/pion/rtp"
)

type Keyframe struct {
	core.Connection
	wr *core.WriteBuffer
}

// Deprecated: should be rewritten
func NewKeyframe() *Keyframe {
	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecJPEG},
				{Name: core.CodecH264},
				{Name: core.CodecH265},
			},
		},
	}
	wr := core.NewWriteBuffer(nil)
	return &Keyframe{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "keyframe",
			Medias:     medias,
			Transport:  wr,
		},
		wr: wr,
	}
}

func (k *Keyframe) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	sender := core.NewSender(media, track.Codec)

	switch track.Codec.Name {
	case core.CodecH264:
		sender.Handler = func(packet *rtp.Packet) {
			if !h264.IsKeyframe(packet.Payload) {
				return
			}
			b := annexb.DecodeAVCC(packet.Payload, true)
			if n, err := k.wr.Write(b); err == nil {
				k.Send += n
			}
		}

		if track.Codec.IsRTP() {
			sender.Handler = h264.RTPDepay(track.Codec, sender.Handler)
		} else {
			sender.Handler = h264.RepairAVCC(track.Codec, sender.Handler)
		}

	case core.CodecH265:
		sender.Handler = func(packet *rtp.Packet) {
			if !h265.IsKeyframe(packet.Payload) {
				return
			}
			b := annexb.DecodeAVCC(packet.Payload, true)
			if n, err := k.wr.Write(b); err == nil {
				k.Send += n
			}
		}

		if track.Codec.IsRTP() {
			sender.Handler = h265.RTPDepay(track.Codec, sender.Handler)
		}

	case core.CodecJPEG:
		sender.Handler = func(packet *rtp.Packet) {
			if n, err := k.wr.Write(packet.Payload); err == nil {
				k.Send += n
			}
		}

		if track.Codec.IsRTP() {
			sender.Handler = mjpeg.RTPDepay(sender.Handler)
		}
	}

	sender.HandleRTP(track)
	k.Senders = append(k.Senders, sender)
	return nil
}

func (k *Keyframe) CodecName() string {
	if len(k.Senders) != 1 {
		return ""
	}
	return k.Senders[0].Codec.Name
}

func (k *Keyframe) WriteTo(wr io.Writer) (int64, error) {
	return k.wr.WriteTo(wr)
}
