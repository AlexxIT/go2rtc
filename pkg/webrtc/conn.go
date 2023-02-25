package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/webrtc/v3"
)

type Conn struct {
	streamer.Element

	UserAgent string

	pc *webrtc.PeerConnection

	medias []*streamer.Media
	tracks []*streamer.Track

	receive int
	send    int

	offer string
	start chan struct{}
}

func NewConn(pc *webrtc.PeerConnection) *Conn {
	c := &Conn{pc: pc, start: make(chan struct{})}

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		c.Fire(candidate)
	})

	pc.OnDataChannel(func(channel *webrtc.DataChannel) {
		c.Fire(channel)
	})

	pc.OnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		track := c.getTrack(remote)
		if track == nil {
			println("ERROR: webrtc: can't find track")
			return
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
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.Fire(state)

		switch state {
		case webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			// disconnect event comes earlier, than failed
			// but it comes only for success connections
			_ = c.Close()
		}
	})

	return c
}

func (c *Conn) Close() error {
	// unblocked write to chan
	select {
	case c.start <- struct{}{}:
	default:
	}
	return c.pc.Close()
}

func (c *Conn) AddCandidate(candidate string) error {
	// pion uses only candidate value from json/object candidate struct
	return c.pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate})
}

func (c *Conn) getTrack(remote *webrtc.TrackRemote) *streamer.Track {
	payloadType := uint8(remote.PayloadType())

	// search existing track (two way audio)
	for _, track := range c.tracks {
		if track.Codec.PayloadType == payloadType {
			return track
		}
	}

	// create new track (incoming WebRTC WHIP)
	for _, media := range c.medias {
		for _, codec := range media.Codecs {
			if codec.PayloadType == payloadType {
				track := streamer.NewTrack(codec, media.Direction)
				c.tracks = append(c.tracks, track)
				return track
			}
		}
	}

	return nil
}

func (c *Conn) remote() string {
	if c.pc == nil {
		return ""
	}

	for _, trans := range c.pc.GetTransceivers() {
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
