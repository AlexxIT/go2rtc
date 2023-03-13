package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

func (c *Conn) SetOffer(offer string) (err error) {
	c.offer = offer

	sd := &sdp.SessionDescription{}
	if err = sd.Unmarshal([]byte(offer)); err != nil {
		return
	}

	// create transceivers with opposite direction
	for _, md := range sd.MediaDescriptions {
		var mid string
		var tr *webrtc.RTPTransceiver
		for _, attr := range md.Attributes {
			switch attr.Key {
			case streamer.DirectionSendRecv:
				tr, _ = c.pc.AddTransceiverFromTrack(NewTrack(md.MediaName.Media))
			case streamer.DirectionSendonly:
				tr, _ = c.pc.AddTransceiverFromKind(
					webrtc.NewRTPCodecType(md.MediaName.Media),
					webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
				)
			case streamer.DirectionRecvonly:
				tr, _ = c.pc.AddTransceiverFromTrack(
					NewTrack(md.MediaName.Media),
					webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly},
				)
			case "mid":
				mid = attr.Value
			}
		}

		if mid != "" && tr != nil {
			_ = tr.SetMid(mid)
		}
	}

	medias := streamer.UnmarshalMedias(sd.MediaDescriptions)

	// sort medias, so video will always be before audio
	// and ignore application media from Hass default lovelace card
	for _, media := range medias {
		if media.Kind == streamer.KindVideo {
			c.medias = append(c.medias, media)
		}
	}
	for _, media := range medias {
		if media.Kind == streamer.KindAudio {
			c.medias = append(c.medias, media)
		}
	}

	return
}

func (c *Conn) GetAnswer() (answer string, err error) {
	// we need to process remote offer after we create transeivers
	desc := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: c.offer}
	if err = c.pc.SetRemoteDescription(desc); err != nil {
		return "", err
	}

	// disable transceivers if we don't have track
	// make direction=inactive
	// don't really necessary, but anyway
	for _, tr := range c.pc.GetTransceivers() {
		if tr.Direction() == webrtc.RTPTransceiverDirectionSendonly && tr.Sender() == nil {
			if err = tr.Stop(); err != nil {
				return
			}
		}
	}

	if desc, err = c.pc.CreateAnswer(nil); err != nil {
		return
	}
	if err = c.pc.SetLocalDescription(desc); err != nil {
		return
	}

	return c.pc.LocalDescription().SDP, nil
}

func (c *Conn) GetCompleteAnswer() (answer string, err error) {
	if _, err = c.GetAnswer(); err != nil {
		return
	}

	<-webrtc.GatheringCompletePromise(c.pc)
	return c.pc.LocalDescription().SDP, nil
}
