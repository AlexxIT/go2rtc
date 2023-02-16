package webrtc

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

// Consumer

func (c *Conn) GetMedias() []*streamer.Media {
	return c.medias
}

func (c *Conn) AddTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	switch track.Direction {
	// send our track to WebRTC consumer
	case streamer.DirectionSendonly:
		codec := track.Codec

		// webrtc.codecParametersFuzzySearch
		caps := webrtc.RTPCodecCapability{
			MimeType:  MimeType(codec),
			Channels:  codec.Channels,
			ClockRate: codec.ClockRate,
		}

		if codec.Name == streamer.CodecH264 {
			// don't know if this really neccessary
			// I have tested multiple browsers and H264 profile has no effect on anything
			caps.SDPFmtpLine = "packetization-mode=1;profile-level-id=42e01f"
		}

		// important to use same streamID so JS will automatically
		// join two tracks as one source/stream
		trackLocal, err := webrtc.NewTrackLocalStaticRTP(
			caps, caps.MimeType[:5], "go2rtc",
		)
		if err != nil {
			return nil
		}

		if _, err = c.Conn.AddTrack(trackLocal); err != nil {
			return nil
		}

		push := func(packet *rtp.Packet) error {
			c.send += packet.MarshalSize()
			return trackLocal.WriteRTP(packet)
		}

		switch codec.Name {
		case streamer.CodecH264:
			wrapper := h264.RTPPay(1200)
			push = wrapper(push)

			if codec.IsRTP() {
				wrapper = h264.RTPDepay(track)
			} else {
				wrapper = h264.RepairAVC(track)
			}
			push = wrapper(push)

		case streamer.CodecH265:
			// SafariPay because it is the only browser in the world
			// that supports WebRTC + H265
			wrapper := h265.SafariPay(1200)
			push = wrapper(push)

			wrapper = h265.RTPDepay(track)
			push = wrapper(push)
		}

		track = track.Bind(push)
		c.tracks = append(c.tracks, track)
		return track

	// receive track from WebRTC consumer (microphone, backchannel, two way audio)
	case streamer.DirectionRecvonly:
		for _, tr := range c.Conn.GetTransceivers() {
			if tr.Mid() != media.MID {
				continue
			}

			codec := track.Codec
			caps := webrtc.RTPCodecCapability{
				MimeType:  MimeType(codec),
				ClockRate: codec.ClockRate,
				Channels:  codec.Channels,
			}
			codecs := []webrtc.RTPCodecParameters{
				{RTPCodecCapability: caps},
			}
			if err := tr.SetCodecPreferences(codecs); err != nil {
				return nil
			}

			c.tracks = append(c.tracks, track)
			return track
		}
	}

	panic("wrong direction")
}

func (c *Conn) MarshalJSON() ([]byte, error) {
	info := &streamer.Info{
		Type:       "WebRTC client",
		RemoteAddr: c.remote(),
		UserAgent:  c.UserAgent,
		Recv:       uint32(c.receive),
		Send:       uint32(c.send),
	}
	return json.Marshal(info)
}
