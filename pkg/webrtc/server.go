package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

type Server struct {
	streamer.Element

	UserAgent string

	conn *webrtc.PeerConnection

	medias []*streamer.Media
	tracks []*streamer.Track

	receive int
	send    int
}

func NewServer(conn *webrtc.PeerConnection) *Server {
	c := &Server{conn: conn}

	conn.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		c.Fire(candidate)
	})

	conn.OnTrack(func(remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		for _, track := range c.tracks {
			if track.Direction != streamer.DirectionRecvonly {
				continue
			}
			if track.Codec.PayloadType != uint8(remote.PayloadType()) {
				continue
			}

			for {
				packet, _, err := remote.ReadRTP()
				if err != nil {
					return
				}
				if len(packet.Payload) == 0 {
					continue
				}
				c.receive += len(packet.Payload)
				_ = track.WriteRTP(packet)
			}
		}

		//fmt.Printf("TODO: webrtc ontrack %+v\n", remote)
	})

	conn.OnDataChannel(func(channel *webrtc.DataChannel) {
		c.Fire(channel)
	})

	// OK connection:
	// 15:01:46 ICE connection state changed: checking
	// 15:01:46 peer connection state changed: connected
	// 15:01:54 peer connection state changed: disconnected
	// 15:02:20 peer connection state changed: failed
	//
	// Fail connection:
	// 14:53:08 ICE connection state changed: checking
	// 14:53:39 peer connection state changed: failed
	conn.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.Fire(state)

		// TODO: remove
		switch state {
		case webrtc.PeerConnectionStateConnected:
			c.Fire(streamer.StatePlaying) // TODO: remove
		case webrtc.PeerConnectionStateDisconnected:
			c.Fire(streamer.StateNull) // TODO: remove
			// disconnect event comes earlier, than failed
			// but it comes only for success connections
			_ = conn.Close()
		case webrtc.PeerConnectionStateFailed:
			_ = conn.Close()
		}
	})

	return c
}

func (c *Server) SetOffer(offer string) (err error) {
	desc := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer, SDP: offer,
	}
	if err = c.conn.SetRemoteDescription(desc); err != nil {
		return
	}

	rawSDP := []byte(c.conn.RemoteDescription().SDP)
	sd := &sdp.SessionDescription{}
	if err = sd.Unmarshal(rawSDP); err != nil {
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

func (c *Server) GetAnswer() (answer string, err error) {
	for _, tr := range c.conn.GetTransceivers() {
		if tr.Direction() != webrtc.RTPTransceiverDirectionSendonly {
			continue
		}

		// disable transceivers if we don't have track
		// make direction=inactive
		// don't really necessary, but anyway
		if tr.Sender() == nil {
			if err = tr.Stop(); err != nil {
				return
			}
		}
	}

	var sdAnswer webrtc.SessionDescription
	sdAnswer, err = c.conn.CreateAnswer(nil)
	if err != nil {
		return
	}

	if err = c.conn.SetLocalDescription(sdAnswer); err != nil {
		return
	}

	return sdAnswer.SDP, nil
}

func (c *Server) GetCompleteAnswer() (answer string, err error) {
	if _, err = c.GetAnswer(); err != nil {
		return
	}

	<-webrtc.GatheringCompletePromise(c.conn)
	return c.conn.LocalDescription().SDP, nil
}

func (c *Server) Close() error {
	return c.conn.Close()
}

func (c *Server) AddCandidate(candidate string) {
	// pion uses only candidate value from json/object candidate struct
	_ = c.conn.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate})
}

func (c *Server) remote() string {
	if c.conn == nil {
		return ""
	}

	for _, trans := range c.conn.GetTransceivers() {
		if trans == nil {
			continue
		}

		receiver := trans.Receiver()
		if receiver == nil {
			continue
		}

		transport := receiver.Transport()
		if transport == nil {
			continue
		}

		iceTransport := transport.ICETransport()
		if iceTransport == nil {
			continue
		}

		pair, _ := iceTransport.GetSelectedCandidatePair()
		if pair == nil || pair.Remote == nil {
			continue
		}

		return pair.Remote.String()
	}

	return ""
}
