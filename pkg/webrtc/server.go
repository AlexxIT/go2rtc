package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
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
			case core.DirectionSendRecv:
				tr, _ = c.pc.AddTransceiverFromTrack(NewTrack(md.MediaName.Media))
			case core.DirectionSendonly:
				tr, _ = c.pc.AddTransceiverFromKind(
					webrtc.NewRTPCodecType(md.MediaName.Media),
					webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
				)
			case core.DirectionRecvonly:
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

	c.Medias = UnmarshalMedias(sd.MediaDescriptions)

	return
}

func (c *Conn) GetAnswer() (answer string, err error) {
	// we need to process remote offer after we create transeivers
	desc := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: c.offer}
	if err = c.pc.SetRemoteDescription(desc); err != nil {
		return "", err
	}

	// disable transceivers if we don't have track, make direction=inactive
transeivers:
	for _, tr := range c.pc.GetTransceivers() {
		for _, sender := range c.Senders {
			if sender.Media.ID == tr.Mid() {
				continue transeivers
			}
		}

		switch tr.Direction() {
		case webrtc.RTPTransceiverDirectionSendrecv:
			_ = tr.Sender().Stop()
		case webrtc.RTPTransceiverDirectionSendonly:
			_ = tr.Stop()
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

// GetCompleteAnswer - get SDP answer with candidates inside
func (c *Conn) GetCompleteAnswer(candidates []string, filter func(*webrtc.ICECandidate) bool) (string, error) {
	var done = make(chan struct{})

	c.pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			if filter == nil || filter(candidate) {
				candidates = append(candidates, candidate.ToJSON().Candidate)
			}
		} else {
			done <- struct{}{}
		}
	})

	answer, err := c.GetAnswer()
	if err != nil {
		return "", err
	}

	<-done

	sd := &sdp.SessionDescription{}
	if err = sd.Unmarshal([]byte(answer)); err != nil {
		return "", err
	}

	md := sd.MediaDescriptions[0]

	for _, candidate := range candidates {
		md.WithPropertyAttribute(candidate)
	}

	b, err := sd.Marshal()
	if err != nil {
		return "", err
	}

	return string(b), nil
}
