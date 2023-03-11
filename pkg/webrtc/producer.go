package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

func (c *Conn) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	if c.Mode != streamer.ModeActiveProducer && c.Mode != streamer.ModePassiveProducer {
		panic("not implemented")
	}

	for _, track := range c.tracks {
		if track.Codec == codec {
			return track
		}
	}

	var track *streamer.Track
	if media.Direction == streamer.DirectionSendonly {
		track = streamer.NewTrack(media, codec)
	} else {
		track = c.getProducerSendTrack(media, codec)
	}

	c.tracks = append(c.tracks, track)
	return track
}

func (c *Conn) Start() error {
	c.closed.Wait()
	return nil
}

func (c *Conn) Stop() error {
	return c.pc.Close()
}

func (c *Conn) getProducerSendTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	tr := c.getTranseiver(media.MID)
	if tr == nil {
		return nil
	}

	sender := tr.Sender()
	if sender == nil {
		return nil
	}

	oldTrack := sender.Track()
	track := &Track{
		kind:        media.Kind,
		payloadType: codec.PayloadType,

		id:       oldTrack.ID(),
		rid:      oldTrack.RID(),
		streamID: oldTrack.StreamID(),
	}

	if err := sender.ReplaceTrack(track); err != nil {
		return nil
	}

	push := func(packet *rtp.Packet) error {
		c.send += packet.MarshalSize()
		return track.WriteRTP(packet)
	}

	return streamer.NewTrack(media, codec).Bind(push)
}

func (c *Conn) getTranseiver(mid string) *webrtc.RTPTransceiver {
	for _, tr := range c.pc.GetTransceivers() {
		if tr.Mid() == mid {
			return tr
		}
	}
	return nil
}

type Track struct {
	kind        string
	id          string
	rid         string
	streamID    string
	payloadType byte
	sequence    uint16
	ssrc        uint32
	writer      webrtc.TrackLocalWriter
}

func (t *Track) Bind(context webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	t.ssrc = uint32(context.SSRC())
	t.writer = context.WriteStream()

	for _, parameters := range context.CodecParameters() {
		if byte(parameters.PayloadType) == t.payloadType {
			return parameters, nil
		}
	}

	return webrtc.RTPCodecParameters{}, nil
}

func (t *Track) Unbind(context webrtc.TrackLocalContext) error {
	return nil
}

func (t *Track) ID() string {
	return t.id
}

func (t *Track) RID() string {
	return t.rid
}

func (t *Track) StreamID() string {
	return t.streamID
}

func (t *Track) Kind() webrtc.RTPCodecType {
	return webrtc.NewRTPCodecType(t.kind)
}

func (t *Track) WriteRTP(packet *rtp.Packet) error {
	// important to have internal counter if input packets from different sources
	t.sequence++

	header := packet.Header
	header.SSRC = t.ssrc
	header.PayloadType = t.payloadType
	header.SequenceNumber = t.sequence
	_, err := t.writer.WriteRTP(&header, packet.Payload)
	return err
}
