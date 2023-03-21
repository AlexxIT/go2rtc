package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/webrtc/v3"
)

func (c *Conn) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	core.Assert(media.Direction == core.DirectionRecvonly)

	for _, track := range c.receivers {
		if track.Codec == codec {
			return track, nil
		}
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

	track := core.NewReceiver(media, codec)
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
