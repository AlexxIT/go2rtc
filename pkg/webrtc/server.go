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
