package webrtc

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/h264"
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

		if codec.Name == streamer.CodecH264 {
			wrapper := h264.RTPPay(1200)
			push = wrapper(push)

			if h264.IsAVC(codec) {
				wrapper = h264.RepairAVC(track)
			} else {
				wrapper = h264.RTPDepay(track)
			}
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

//

func (c *Conn) Push(msg interface{}) {
	if msg := msg.(*streamer.Message); msg != nil {
		if msg.Type == MsgTypeCandidate {
			_ = c.Conn.AddICECandidate(webrtc.ICECandidateInit{
				Candidate: msg.Value.(string),
			})
		}
	}
}

func (c *Conn) MarshalJSON() ([]byte, error) {
	v := map[string]interface{}{
		streamer.JSONType:       "WebRTC server consumer",
		streamer.JSONRemoteAddr: c.remote(),
	}

	if c.receive > 0 {
		v[streamer.JSONReceive] = c.receive
	}
	if c.send > 0 {
		v[streamer.JSONSend] = c.send
	}
	if c.UserAgent != "" {
		v[streamer.JSONUserAgent] = c.UserAgent
	}

	return json.Marshal(v)
}
