package probe

import (
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
)

type Probe struct {
	core.Connection
}

func Create(name string, query url.Values) *Probe {
	medias := core.ParseQuery(query)

	for _, value := range query["microphone"] {
		media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly}

		for _, name := range strings.Split(value, ",") {
			name = strings.ToUpper(name)
			switch name {
			case "", "COPY":
				name = core.CodecAny
			}
			media.Codecs = append(media.Codecs, &core.Codec{Name: name})
		}

		medias = append(medias, media)
	}

	return &Probe{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: name,
			Medias:     medias,
		},
	}
}

func (p *Probe) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	sender := core.NewSender(media, track.Codec)

	handler := func(pkt *core.Packet) {
		p.Send += len(pkt.Payload)
	}

	// Apply format handlers to update FmtpLine from first keyframe
	// This fixes MSE aspect ratio issues when RTSP cameras don't send SPS/PPS in DESCRIBE
	switch track.Codec.Name {
	case core.CodecH264:
		if track.Codec.IsRTP() {
			handler = h264.RTPDepay(track.Codec, handler)
		} else {
			handler = h264.RepairAVCC(track.Codec, handler)
		}
	case core.CodecH265:
		if track.Codec.IsRTP() {
			handler = h265.RTPDepay(track.Codec, handler)
		} else {
			handler = h265.RepairAVCC(track.Codec, handler)
		}
	}

	sender.Handler = handler
	sender.HandleRTP(track)
	p.Senders = append(p.Senders, sender)
	return nil
}

func (p *Probe) Start() error {
	return nil
}
