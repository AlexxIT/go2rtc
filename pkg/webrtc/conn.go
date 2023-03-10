package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/webrtc/v3"
)

type Conn struct {
	streamer.Element

	UserAgent string
	Desc      string
	Mode      streamer.Mode

	pc *webrtc.PeerConnection

	medias []*streamer.Media
	tracks []*streamer.Track

	receive int
	send    int

	offer  string
	remote string
	closed core.Waiter
}

func NewConn(pc *webrtc.PeerConnection) *Conn {
	c := &Conn{pc: pc}

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		// last candidate will be empty
		if candidate != nil {
			c.Fire(candidate)
		}
	})

	pc.OnDataChannel(func(channel *webrtc.DataChannel) {
		c.Fire(channel)
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state != webrtc.ICEConnectionStateChecking {
			return
		}
		pc.SCTP().Transport().ICETransport().OnSelectedCandidatePairChange(
			func(pair *webrtc.ICECandidatePair) {
				c.remote = pair.Remote.String()
			},
		)
	})

	pc.OnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		track := c.getRecvTrack(remote)
		if track == nil {
			return // it's OK when we not need, for example, audio from producer
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
	c.closed.Done()
	return c.pc.Close()
}

func (c *Conn) AddCandidate(candidate string) error {
	// pion uses only candidate value from json/object candidate struct
	return c.pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate})
}

func (c *Conn) getRecvTrack(remote *webrtc.TrackRemote) *streamer.Track {
	payloadType := uint8(remote.PayloadType())

	switch c.Mode {
	// browser microphone (backchannel)
	case streamer.ModePassiveConsumer:
		for _, track := range c.tracks {
			if track.Direction == streamer.DirectionRecvonly && track.Codec.PayloadType == payloadType {
				return track
			}
		}

	case streamer.ModeActiveProducer:
		// remote track from WebRTC active producer (audio/video)
		for _, track := range c.tracks {
			if track.Direction == streamer.DirectionSendonly && track.Codec.PayloadType == payloadType {
				return track
			}
		}

	case streamer.ModePassiveProducer:
		// remote track from WebRTC passive producer (incoming WebRTC WHIP)
		for i, media := range c.medias {
			for _, codec := range media.Codecs {
				if codec.PayloadType != payloadType {
					continue
				}

				// leave only one codec in supported media list
				if len(media.Codecs) > 1 {
					c.medias[i].Codecs = []*streamer.Codec{codec}
				}

				track := streamer.NewTrack(media, codec)
				c.tracks = append(c.tracks, track)
				return track
			}
		}

	default:
		panic("not implemented")
	}

	return nil
}
