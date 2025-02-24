package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

func (c *Conn) CreateOffer(medias []*core.Media) (string, error) {
	// 1. Create transeivers with proper kind and direction
	for _, media := range medias {
		var err error
		switch media.Direction {
		case core.DirectionRecvonly:
			_, err = c.pc.AddTransceiverFromKind(
				webrtc.NewRTPCodecType(media.Kind),
				webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
			)
		case core.DirectionSendonly:
			_, err = c.pc.AddTransceiverFromTrack(
				NewTrack(media.Kind),
				webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly},
			)
		case core.DirectionSendRecv:
			// default transceiver is sendrecv
			_, err = c.pc.AddTransceiverFromTrack(NewTrack(media.Kind))
		default:
			// Nest cameras require data channel
			_, err = c.pc.CreateDataChannel(media.Kind, nil)
		}

		if err != nil {
			return "", err
		}
	}

	// 2. Create local offer
	desc, err := c.pc.CreateOffer(nil)
	if err != nil {
		return "", err
	}

	// 3. Start gathering phase
	if err = c.pc.SetLocalDescription(desc); err != nil {
		return "", err
	}

	return c.pc.LocalDescription().SDP, nil
}

func (c *Conn) CreateCompleteOffer(medias []*core.Media) (string, error) {
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

	c.Medias = UnmarshalMedias(sd.MediaDescriptions)

	return nil
}

// fakeFormatsInAnswer - fix pion bug with remote SDP parsing:
// pion will process formats only from first media of each kind
// so we add all formats from first offer media to the first answer media
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
