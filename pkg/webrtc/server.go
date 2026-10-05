package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
)

func (c *Conn) SetOffer(offer string) (err error) {
	c.offer = offer

	sd := &sdp.SessionDescription{}
	if err = sd.Unmarshal([]byte(offer)); err != nil {
		return
	}

	// create transceivers with opposite direction
	for _, md := range sd.MediaDescriptions {
		c.addTransceiver(md)
	}

	c.Medias = UnmarshalMedias(sd.MediaDescriptions)

	return
}

// SetReOffer - process new offer from the same remote peer for established connection (renegotiation).
// Returns only new medias, that should be matched with the stream. For example, remote peer changes
// audio direction from recvonly to sendrecv for enabling two-way audio.
func (c *Conn) SetReOffer(offer string) (medias []*core.Media, err error) {
	sd := &sdp.SessionDescription{}
	if err = sd.Unmarshal([]byte(offer)); err != nil {
		return
	}

	// pion reuses transceivers with the same mid, so create transceivers only for new mids
	for _, md := range sd.MediaDescriptions {
		if mid, _ := md.Attribute("mid"); mid == "" || c.getTranseiver(mid) == nil {
			c.addTransceiver(md)
		}
	}

	// keep old medias, because senders and receivers are linked to them,
	// so we can reuse them if remote peer disables and enables direction again
	for _, media := range UnmarshalMedias(sd.MediaDescriptions) {
		if c.getMedia(media.ID, media.Direction) == nil {
			medias = append(medias, media)
		}
	}

	c.offer = offer
	c.Medias = append(c.Medias, medias...)

	return
}

// IsReOffer - check if offer comes from the same remote peer (same DTLS fingerprint)
// for established connection
func (c *Conn) IsReOffer(offer string) bool {
	if c.pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
		return false
	}

	remote := c.pc.RemoteDescription()
	if remote == nil {
		return false
	}

	fingerprint := getFingerprint(offer)
	return fingerprint != "" && fingerprint == getFingerprint(remote.SDP)
}

func (c *Conn) addTransceiver(md *sdp.MediaDescription) {
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
			// create sendrecv transceiver, so it will have receiver, pion will change direction
			// to sendonly in SetRemoteDescription, and remote peer can change direction
			// to sendrecv later with renegotiation (two-way audio)
			tr, _ = c.pc.AddTransceiverFromTrack(NewTrack(md.MediaName.Media))
		case "mid":
			mid = attr.Value
		}
	}

	if mid != "" && tr != nil {
		_ = tr.SetMid(mid)
	}
}

func (c *Conn) getMedia(mid, direction string) *core.Media {
	for _, media := range c.Medias {
		if media.ID == mid && media.Direction == direction {
			return media
		}
	}
	return nil
}

func getFingerprint(offer string) string {
	sd := &sdp.SessionDescription{}
	if err := sd.Unmarshal([]byte(offer)); err != nil {
		return ""
	}
	if fingerprint, ok := sd.Attribute("fingerprint"); ok {
		return fingerprint
	}
	for _, md := range sd.MediaDescriptions {
		if fingerprint, ok := md.Attribute("fingerprint"); ok {
			return fingerprint
		}
	}
	return ""
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
		// transceiver without sender is already recvonly or inactive
		if tr.Sender() == nil {
			continue
		}

		for _, sender := range c.Senders {
			if sender.Media.ID == tr.Mid() {
				continue transeivers
			}
		}

		switch tr.Direction() {
		case webrtc.RTPTransceiverDirectionSendrecv, webrtc.RTPTransceiverDirectionSendonly:
			// don't use tr.Stop(), because it also stops receiver
			// and remote peer can't enable sending (backchannel) with renegotiation
			_ = tr.Sender().Stop()             // don't know if necessary
			_ = tr.SetSender(tr.Sender(), nil) // sendrecv => recvonly, sendonly => inactive
		}
	}

	if desc, err = c.pc.CreateAnswer(nil); err != nil {
		return
	}
	if err = c.pc.SetLocalDescription(desc); err != nil {
		return
	}

	// start new senders after renegotiation, because connected state already passed
	if c.pc.ConnectionState() == webrtc.PeerConnectionStateConnected {
		for _, sender := range c.Senders {
			sender.Start()
		}
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
