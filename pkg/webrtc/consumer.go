package webrtc

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

func (c *Conn) GetMedias() []*core.Media {
	return c.medias
}

func (c *Conn) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	core.Assert(media.Direction == core.DirectionSendonly)

	for _, sender := range c.senders {
		if sender.Codec == codec {
			sender.HandleRTP(track)
			return nil
		}
	}

	switch c.Mode {
	case core.ModePassiveConsumer: // video/audio for browser
	case core.ModeActiveProducer: // go2rtc as WebRTC client (backchannel)
	case core.ModePassiveProducer: // WebRTC/WHIP
	default:
		panic(core.Caller())
	}

	localTrack := c.getTranseiver(media.ID).Sender().Track().(*Track)

	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		c.send += packet.MarshalSize()
		//important to send with remote PayloadType
		_ = localTrack.WriteRTP(codec.PayloadType, packet)
	}

	switch codec.Name {
	case core.CodecH264:
		sender.Handler = h264.RTPPay(1200, sender.Handler)
		if track.Codec.IsRTP() {
			sender.Handler = h264.RTPDepay(track.Codec, sender.Handler)
		} else {
			sender.Handler = h264.RepairAVC(track.Codec, sender.Handler)
		}

	case core.CodecH265:
		// SafariPay because it is the only browser in the world
		// that supports WebRTC + H265
		sender.Handler = h265.SafariPay(1200, sender.Handler)
		if track.Codec.IsRTP() {
			sender.Handler = h265.RTPDepay(track.Codec, sender.Handler)
		}
	}

	sender.HandleRTP(track)

	c.senders = append(c.senders, sender)
	return nil
}

func (c *Conn) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:       c.Desc + " " + c.Mode.String(),
		RemoteAddr: c.remote,
		UserAgent:  c.UserAgent,
		Medias:     c.medias,
		Receivers:  c.receivers,
		Senders:    c.senders,
		Recv:       c.recv,
		Send:       c.send,
	}
	return json.Marshal(info)
}
