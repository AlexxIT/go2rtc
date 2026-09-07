package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/webrtc/v4"
)

func (c *Conn) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	core.Assert(media.Direction == core.DirectionRecvonly)

	for _, track := range c.Receivers {
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
				Channels:  uint16(codec.Channels),
			},
			PayloadType: 0, // don't know if this necessary
		}

		tr := c.getTranseiver(media.ID)

		_ = tr.SetCodecPreferences([]webrtc.RTPCodecParameters{params})

	case core.ModePassiveProducer, core.ModeActiveProducer:
		// Passive producers: OBS Studio via WHIP or Browser
		// Active producers: go2rtc as WebRTC client or WebTorrent

		// Some remote peers (observed with Nest/SDM WebRTC) start sending
		// media on a different payload type than the one originally wired
		// to consumers (e.g. switch from PT 96 to PT 98 mid-session for the
		// same video media, without a renegotiation). Since each payload
		// type is a distinct *Codec pointer, the exact-codec lookup above
		// misses and a brand-new, unwired Receiver would otherwise be
		// created for it, silently starving every already-connected
		// consumer. Reuse the existing Receiver for this Media instead, so
		// already-wired Senders keep receiving packets regardless of which
		// declared payload type the remote actually transmits on.
		for _, track := range c.Receivers {
			if track.Media == media {
				return track, nil
			}
		}

	default:
		panic(core.Caller())
	}

	track := core.NewReceiver(media, codec)
	c.Receivers = append(c.Receivers, track)
	return track, nil
}

func (c *Conn) Start() error {
	c.closed.Wait()
	return nil
}
