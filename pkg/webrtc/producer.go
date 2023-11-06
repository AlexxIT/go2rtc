package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/webrtc/v3"
)

func (c *Conn) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	core.Assert(media.Direction == core.DirectionRecvonly)

	for _, track := range c.receivers {
		if track.Codec.Match(codec) {
			return track, nil
		}
	}

	track := core.NewReceiver(media, codec)

	if codec.ClockRate == 0 {
		if codec.Name == core.CodecPCM || codec.Name == core.CodecPCML {
			codec.Name = core.CodecPCMA
		}
		codec.ClockRate = 8000
		track.Handler = pcm.ResampleToPCMA(track.Codec, 16000, track.Handler) //TODO
	}

	switch c.Mode {
	case core.ModePassiveConsumer: // backchannel from browser
		// set codec for consumer recv track so remote peer should send media with this codec
		params := webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  MimeType(codec),
				ClockRate: codec.ClockRate,
				Channels:  codec.Channels,
			},
			PayloadType: 0, // don't know if this necessary
		}

		tr := c.getTranseiver(media.ID)

		_ = tr.SetCodecPreferences([]webrtc.RTPCodecParameters{params})

	case core.ModePassiveProducer, core.ModeActiveProducer:
		// Passive producers: OBS Studio via WHIP or Browser
		// Active producers: go2rtc as WebRTC client or WebTorrent

	default:
		panic(core.Caller())
	}

	c.receivers = append(c.receivers, track)
	return track, nil
}

func (c *Conn) Start() error {
	c.closed.Wait()
	return nil
}

func (c *Conn) Stop() error {
	for _, receiver := range c.receivers {
		receiver.Close()
	}
	for _, sender := range c.senders {
		sender.Close()
	}
	return c.pc.Close()
}
