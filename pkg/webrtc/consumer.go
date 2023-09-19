package webrtc

import (
	"encoding/json"
	"errors"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

func (c *Conn) GetMedias() []*core.Media {
	return WithResampling(c.medias)
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

	localTrack := c.getSenderTrack(media.ID)
	if localTrack == nil {
		return errors.New("webrtc: can't get track")
	}

	payloadType := codec.PayloadType

	sender := core.NewSender(media, codec)
	sender.Handler = func(packet *rtp.Packet) {
		c.send += packet.MarshalSize()
		//important to send with remote PayloadType
		_ = localTrack.WriteRTP(payloadType, packet)
	}

	switch track.Codec.Name {
	case core.CodecH264:
		sender.Handler = h264.RTPPay(1200, sender.Handler)
		if track.Codec.IsRTP() {
			sender.Handler = h264.RTPDepay(track.Codec, sender.Handler)
		} else {
			sender.Handler = h264.RepairAVCC(track.Codec, sender.Handler)
		}

	case core.CodecH265:
		// SafariPay because it is the only browser in the world
		// that supports WebRTC + H265
		sender.Handler = h265.SafariPay(1200, sender.Handler)
		if track.Codec.IsRTP() {
			sender.Handler = h265.RTPDepay(track.Codec, sender.Handler)
		}

	case core.CodecPCMA, core.CodecPCMU, core.CodecPCM, core.CodecPCML:
		if codec.ClockRate == 0 {
			if codec.Name == core.CodecPCM || codec.Name == core.CodecPCML {
				codec.Name = core.CodecPCMA
			}
			codec.ClockRate = 8000
			sender.Handler = pcm.ResampleToG711(track.Codec, 8000, sender.Handler)
		}

		// Fix audio quality https://github.com/AlexxIT/WebRTC/issues/500
		sender.Handler = pcm.RepackG711(false, sender.Handler)
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
