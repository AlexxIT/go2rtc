package webrtc

import "github.com/pion/webrtc/v3"

func (c *Conn) CreateOffer() (string, error) {
	init := webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}
	_, _ = c.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, init)
	_, _ = c.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, init)

	desc, err := c.pc.CreateOffer(nil)
	if err != nil {
		return "", err
	}

	if err = c.pc.SetLocalDescription(desc); err != nil {
		return "", err
	}

	return desc.SDP, nil
}

func (c *Conn) CreateCompleteOffer() (string, error) {
	if _, err := c.CreateOffer(); err != nil {
		return "", err
	}

	<-webrtc.GatheringCompletePromise(c.pc)
	return c.pc.LocalDescription().SDP, nil
}

func (c *Conn) SetAnswer(answer string) (err error) {
	desc := webrtc.SessionDescription{SDP: answer, Type: webrtc.SDPTypeAnswer}
	return c.pc.SetRemoteDescription(desc)
}
