package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
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
		if track == nil {
			panic("getProducerSendTrack return nil track")
		}
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

	track, ok := sender.Track().(*Track)
	if !ok {
		return nil
	}

	push := func(packet *rtp.Packet) error {
		c.send += packet.MarshalSize()
		return track.WriteRTP(codec.PayloadType, packet)
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
	kind     string
	id       string
	streamID string
	sequence uint16
	ssrc     uint32
	writer   webrtc.TrackLocalWriter
}

func NewTrack(kind string) *Track {
	return &Track{
		kind:     kind,
		id:       core.RandString(16),
		streamID: core.RandString(16),
	}
}

func (t *Track) Bind(context webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	t.ssrc = uint32(context.SSRC())
	t.writer = context.WriteStream()

	for _, parameters := range context.CodecParameters() {
		// return first parameters
		return parameters, nil
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
	return "" // don't know what it is
}

func (t *Track) StreamID() string {
	return t.streamID
}

func (t *Track) Kind() webrtc.RTPCodecType {
	return webrtc.NewRTPCodecType(t.kind)
}

func (t *Track) WriteRTP(payloadType uint8, packet *rtp.Packet) error {
	// important to have internal counter if input packets from different sources
	t.sequence++

	header := packet.Header
	header.SSRC = t.ssrc
	header.PayloadType = payloadType
	header.SequenceNumber = t.sequence
	_, err := t.writer.WriteRTP(&header, packet.Payload)
	return err
}
