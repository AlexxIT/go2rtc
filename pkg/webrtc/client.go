package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

func (c *Conn) CreateOffer(medias []*streamer.Media) (string, error) {
	for _, media := range medias {
		if _, err := c.pc.AddTransceiverFromKind(
			webrtc.NewRTPCodecType(media.Kind), webrtc.RTPTransceiverInit{
				Direction: webrtc.NewRTPTransceiverDirection(media.Direction),
			},
		); err != nil {
			return "", err
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
	desc := webrtc.SessionDescription{SDP: answer, Type: webrtc.SDPTypeAnswer}
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
