package homekit

import (
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/opus"
	"github.com/pion/rtp"
)

// HomeKit cameras and doorbells with a speaker play the audio a controller
// sends on the stream's audio session. They take it only in the audio codec
// selected for the stream, so the backchannel offers just the codecs the
// camera supports, and one codec is used in both directions.

const typeSpeaker = "113"

// backchannelMedia returns the sendonly media for an accessory with a
// speaker, or nil.
func backchannelMedia(acc *hap.Accessory, codecs []camera.AudioCodecConfiguration) *core.Media {
	if acc.GetService(typeSpeaker) == nil {
		return nil
	}

	media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionSendonly}

	var hasOpus bool
	for _, codec := range audioToMedia(codecs).Codecs {
		switch codec.Name {
		case core.CodecOpus:
			if !hasOpus {
				// Opus RTP always uses a 48 kHz clock (RFC 7587), whatever
				// the real sample rate. HAP timestamps are fixed on sending.
				media.Codecs = append(media.Codecs, &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2})
				hasOpus = true
			}
		case core.CodecELD:
			media.Codecs = append(media.Codecs, codec)
		}
	}

	if len(media.Codecs) == 0 {
		return nil
	}
	return media
}

// backchannelToAudio returns the stream's audio configuration when only the
// backchannel decides the codec: the camera's own entry for that codec,
// preferring 16 kHz.
func backchannelToAudio(name string, codecs []camera.AudioCodecConfiguration) *camera.AudioCodecConfiguration {
	for _, codec := range codecs {
		if audioCodecs[codec.CodecType] != name {
			continue
		}
		conf := codec
		params := conf.CodecParams[0]
		for _, rate := range params.SampleRate {
			if rate == camera.AudioCodecSampleRate16Khz {
				params.SampleRate = []byte{rate}
				break
			}
		}
		conf.CodecParams = []camera.AudioCodecParameters{params}
		return trackToAudio(nil, &conf)
	}
	return nil
}

// AddTrack sends a backchannel track (e.g. a browser's microphone) to the
// camera's speaker over the stream's audio session.
func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	c.mu.Lock()
	c.backchannelCodec = codec.Name
	c.updateBackchannel()
	c.mu.Unlock()

	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		// set once the stream has started with the same audio codec
		if session := c.backchannelSession.Load(); session != nil {
			if n, err := session.WriteRTP(packet); err == nil {
				c.Send += n
			}
		}
	}

	switch codec.Name {
	case core.CodecOpus:
		// trackToAudio selects 20 ms packets for Opus
		sender.Handler = opus.RepackToHAP(20, sender.Handler)
	case core.CodecELD:
		// HomeKit AAC-ELD frames are 480 samples, one per packet
		sender.Handler = aac.RTPPaySamples(480, sender.Handler)
		if track.Codec.IsRTP() {
			sender.Handler = aac.RTPDepay(sender.Handler)
		}
	}

	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}
