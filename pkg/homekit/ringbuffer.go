package homekit

import (
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

// Packet is one media sample held in the recording pre-buffer
type Packet struct {
	Track   byte // 0=video, 1=audio
	Codec   *core.Codec
	Payload []byte
	RTPTime uint32
	Wall    time.Time
	Key     bool
}

// RingBuffer keeps a wall-clock window of recent media for CMAF clip export
type RingBuffer struct {
	mu         sync.Mutex
	packets    []Packet
	maxAge     time.Duration
	videoCodec *core.Codec
	audioCodec *core.Codec
}

// NewRingBuffer creates a buffer retaining maxAge of media (e.g. 8s pre-roll)
func NewRingBuffer(maxAge time.Duration) *RingBuffer {
	if maxAge <= 0 {
		maxAge = 8 * time.Second
	}
	return &RingBuffer{maxAge: maxAge}
}

// Push appends a packet and drops samples older than maxAge
func (b *RingBuffer) Push(p Packet) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if p.Track == 0 {
		b.videoCodec = p.Codec
	} else {
		b.audioCodec = p.Codec
	}

	// copy payload so caller may reuse the buffer
	p.Payload = append([]byte(nil), p.Payload...)
	b.packets = append(b.packets, p)
	b.trimLocked(time.Now())
}

func (b *RingBuffer) trimLocked(now time.Time) {
	cut := now.Add(-b.maxAge)
	i := 0
	for i < len(b.packets) && b.packets[i].Wall.Before(cut) {
		i++
	}
	if i > 0 {
		b.packets = append([]Packet(nil), b.packets[i:]...)
	}
}

// Codecs returns the latest video/audio codecs seen
func (b *RingBuffer) Codecs() (video, audio *core.Codec) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.videoCodec, b.audioCodec
}

// Slice returns packets with Wall in [start, end] inclusive, starting at a keyframe when possible
func (b *RingBuffer) Slice(start, end time.Time) []Packet {
	b.mu.Lock()
	defer b.mu.Unlock()

	if end.IsZero() {
		end = time.Now()
	}
	if start.IsZero() {
		start = end.Add(-b.maxAge)
	}

	// find first video keyframe at or before start
	keyIdx := -1
	for i, p := range b.packets {
		if p.Track == 0 && p.Key && !p.Wall.After(start) {
			keyIdx = i
		}
	}
	from := 0
	if keyIdx >= 0 {
		from = keyIdx
	} else {
		// fall back to first packet not after start
		for i, p := range b.packets {
			if !p.Wall.Before(start) {
				from = i
				break
			}
		}
	}

	var out []Packet
	for i := from; i < len(b.packets); i++ {
		p := b.packets[i]
		if p.Wall.After(end) {
			break
		}
		cp := p
		cp.Payload = append([]byte(nil), p.Payload...)
		out = append(out, cp)
	}
	return out
}

// Len returns buffered packet count
func (b *RingBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.packets)
}

// OldestWall returns the oldest packet time (zero if empty)
func (b *RingBuffer) OldestWall() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.packets) == 0 {
		return time.Time{}
	}
	return b.packets[0].Wall
}

// BufferConsumer is a core.Consumer that fills a RingBuffer from a stream
type BufferConsumer struct {
	core.Connection
	buf *RingBuffer
}

// NewBufferConsumer creates a consumer that records into buf
func NewBufferConsumer(buf *RingBuffer) *BufferConsumer {
	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecH265},
				{Name: core.CodecH264},
			},
		},
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecOpus, ClockRate: 48000, Channels: 2},
				{Name: core.CodecAAC},
			},
		},
	}
	return &BufferConsumer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "homekit/buffer",
			Protocol:   "internal",
			Medias:     medias,
		},
		buf: buf,
	}
}

// AddTrack registers a media track into the ring buffer
func (c *BufferConsumer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	codec := track.Codec.Clone()
	trackID := byte(0)
	if codec.Kind() == core.KindAudio {
		trackID = 1
	}

	sender := core.NewSender(media, codec)
	sender.Handler = func(packet *rtp.Packet) {
		key := false
		switch codec.Name {
		case core.CodecH264:
			key = h264.IsKeyframe(packet.Payload)
		case core.CodecH265:
			key = h265.IsKeyframe(packet.Payload)
		}
		c.buf.Push(Packet{
			Track:   trackID,
			Codec:   codec,
			Payload: packet.Payload,
			RTPTime: packet.Timestamp,
			Wall:    time.Now(),
			Key:     key,
		})
		c.Send += len(packet.Payload)
	}

	switch codec.Name {
	case core.CodecH264:
		if track.Codec.IsRTP() {
			sender.Handler = h264.RTPDepay(track.Codec, sender.Handler)
		} else {
			sender.Handler = h264.RepairAVCC(track.Codec, sender.Handler)
		}
	case core.CodecH265:
		if track.Codec.IsRTP() {
			sender.Handler = h265.RTPDepay(track.Codec, sender.Handler)
		} else {
			sender.Handler = h265.RepairAVCC(track.Codec, sender.Handler)
		}
	}

	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}
