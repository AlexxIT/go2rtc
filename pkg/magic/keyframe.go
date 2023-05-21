package magic

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/pion/rtp"
)

type Keyframe struct {
	core.Listener

	UserAgent  string
	RemoteAddr string

	medias []*core.Media
	sender *core.Sender
}

func (k *Keyframe) GetMedias() []*core.Media {
	if k.medias == nil {
		k.medias = append(k.medias, &core.Media{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecH264},
				{Name: core.CodecH265},
				{Name: core.CodecJPEG},
			},
		})
	}
	return k.medias
}

func (k *Keyframe) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	var handler core.HandlerFunc

	switch track.Codec.Name {
	case core.CodecH264:
		handler = func(packet *rtp.Packet) {
			if !h264.IsKeyframe(packet.Payload) {
				return
			}
			b := h264.AVCtoAnnexB(packet.Payload)
			k.Fire(b)
		}

		if track.Codec.IsRTP() {
			handler = h264.RTPDepay(track.Codec, handler)
		}
	case core.CodecH265:
		handler = func(packet *rtp.Packet) {
			if !h265.IsKeyframe(packet.Payload) {
				return
			}
			k.Fire(packet.Payload)
		}

		if track.Codec.IsRTP() {
			handler = h265.RTPDepay(track.Codec, handler)
		}
	case core.CodecJPEG:
		handler = func(packet *rtp.Packet) {
			k.Fire(packet.Payload)
		}

		if track.Codec.IsRTP() {
			handler = mjpeg.RTPDepay(handler)
		}
	}

	k.sender = core.NewSender(media, track.Codec)
	k.sender.Handler = handler
	k.sender.HandleRTP(track)
	return nil
}

func (k *Keyframe) CodecName() string {
	if k.sender != nil {
		return k.sender.Codec.Name
	}
	return ""
}

func (k *Keyframe) Stop() error {
	if k.sender != nil {
		k.sender.Close()
	}
	return nil
}
