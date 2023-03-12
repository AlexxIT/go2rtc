package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

func (c *Conn) CreateOffer(medias []*streamer.Media) (string, error) {
	for _, media := range medias {
		switch media.Direction {
		case streamer.DirectionRecvonly:
			if _, err := c.pc.AddTransceiverFromKind(
				webrtc.NewRTPCodecType(media.Kind),
				webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
			); err != nil {
				return "", err
			}
		case streamer.DirectionSendonly:
			if _, err := c.pc.AddTransceiverFromTrack(
				NewTrack(media.Kind),
				webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly},
			); err != nil {
				return "", err
			}
		case streamer.DirectionSendRecv:
			panic("not implemented")
		}
	}

	desc, err := c.pc.CreateOffer(nil)
	if err != nil {
		return "", err
	}

	if err = c.pc.SetLocalDescription(desc); err != nil {
		return "", err
	}

	return c.pc.LocalDescription().SDP, nil
}

func (c *Conn) CreateCompleteOffer(medias []*streamer.Media) (string, error) {
	if _, err := c.CreateOffer(medias); err != nil {
		return "", err
	}

	<-webrtc.GatheringCompletePromise(c.pc)
	return c.pc.LocalDescription().SDP, nil
}

func (c *Conn) SetAnswer(answer string) (err error) {
	desc := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  fakeFormatsInAnswer(c.pc.LocalDescription().SDP, answer),
	}
	if err = c.pc.SetRemoteDescription(desc); err != nil {
		return
	}

	sd := &sdp.SessionDescription{}
	if err = sd.Unmarshal([]byte(answer)); err != nil {
		return
	}

	medias := streamer.UnmarshalMedias(sd.MediaDescriptions)

	// sort medias, so video will always be before audio
	// and ignore application media from Hass default lovelace card
	// ignore media without direction (inactive media)
	for _, media := range medias {
		if media.Kind == streamer.KindVideo && media.Direction != "" {
			c.medias = append(c.medias, media)
		}
	}
	for _, media := range medias {
		if media.Kind == streamer.KindAudio && media.Direction != "" {
			c.medias = append(c.medias, media)
		}
	}

	return nil
}

func fakeFormatsInAnswer(offer, answer string) string {
	sd2 := &sdp.SessionDescription{}
	if err := sd2.Unmarshal([]byte(answer)); err != nil {
		return answer
	}

	// check if answer has recvonly audio
	var ok bool
	for _, md2 := range sd2.MediaDescriptions {
		if md2.MediaName.Media == "audio" {
			if _, ok = md2.Attribute("recvonly"); ok {
				break
			}
		}
	}
	if !ok {
		return answer
	}

	sd1 := &sdp.SessionDescription{}
	if err := sd1.Unmarshal([]byte(offer)); err != nil {
		return answer
	}

	var formats []string
	var attrs []sdp.Attribute

	for _, md1 := range sd1.MediaDescriptions {
		if md1.MediaName.Media == "audio" {
			for _, attr := range md1.Attributes {
				switch attr.Key {
				case "rtpmap", "fmtp", "rtcp-fb", "extmap":
					attrs = append(attrs, attr)
				}
			}

			formats = md1.MediaName.Formats
			break
		}
	}

	for _, md2 := range sd2.MediaDescriptions {
		if md2.MediaName.Media == "audio" {
			for _, attr := range md2.Attributes {
				switch attr.Key {
				case "rtpmap", "fmtp", "rtcp-fb", "extmap":
				default:
					attrs = append(attrs, attr)
				}
			}

			md2.MediaName.Formats = formats
			md2.Attributes = attrs
			break
		}
	}

	b, err := sd2.Marshal()
	if err != nil {
		return answer
	}

	return string(b)
}
