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
		//
		// The remote may answer one media with several payload types and then
		// transmit on one that is not the first (Google Nest/SDM answers an
		// H264 offer with two payload types and streams on the second). A
		// consumer that attached before the first RTP packet already owns a
		// Receiver for the first codec, so OnTrack must reuse it instead of
		// creating a second Receiver that no consumer is wired to. Only a
		// compatible codec is adopted (same name, clock rate and channels);
		// a switch to a different codec still gets its own Receiver.
		for _, track := range c.Receivers {
			if track.Media == media && track.Codec.Match(codec) {
				track.Codec = codec
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
