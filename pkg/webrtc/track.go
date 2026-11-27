package webrtc

import (
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type Track struct {
	kind     string
	id       string
	streamID string
	sequence uint16
	ssrc     uint32
	writer   webrtc.TrackLocalWriter
	sender   *core.Sender // asked for a keyframe on PLI/FIR
	mu       sync.Mutex
}

func NewTrack(kind string) *Track {
	return &Track{
		kind:     kind,
		id:       "go2rtc-" + kind,
		streamID: "go2rtc",
	}
}

func (t *Track) Bind(context webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	t.mu.Lock()
	t.ssrc = uint32(context.SSRC())
	t.writer = context.WriteStream()
	t.mu.Unlock()

	for _, parameters := range context.CodecParameters() {
		// return first parameters
		return parameters, nil
	}

	return webrtc.RTPCodecParameters{}, nil
}

func (t *Track) Unbind(context webrtc.TrackLocalContext) error {
	t.mu.Lock()
	t.writer = nil
	t.mu.Unlock()
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

func (t *Track) WriteRTP(payloadType uint8, packet *rtp.Packet) (err error) {
	// using mutex because Unbind https://github.com/AlexxIT/go2rtc/issues/994
	t.mu.Lock()

	// in case when we start WriteRTP before Track.Bind
	if t.writer != nil {
		// important to have internal counter if input packets from different sources
		t.sequence++

		header := packet.Header
		header.SSRC = t.ssrc
		header.PayloadType = payloadType
		header.SequenceNumber = t.sequence
		_, err = t.writer.WriteRTP(&header, packet.Payload)
	}

	t.mu.Unlock()
	return
}

func (t *Track) setSender(sender *core.Sender) {
	t.mu.Lock()
	t.sender = sender
	t.mu.Unlock()
}

// requestKeyframe asks the source of the track for a keyframe, the source is
// looked up on each call, because AddTrack can bind the sender to a new one
func (t *Track) requestKeyframe() {
	t.mu.Lock()
	sender := t.sender
	t.mu.Unlock()

	if sender != nil {
		sender.RequestKeyframe()
	}
}
