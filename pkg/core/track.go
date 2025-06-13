package core

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/pion/rtp"
)

var ErrCantGetTrack = errors.New("can't get track")

type Receiver struct {
	Node

	// Deprecated: should be removed
	Media *Media `json:"-"`
	// Deprecated: should be removed
	ID byte `json:"-"` // Channel for RTSP, PayloadType for MPEG-TS

	Bytes   int `json:"bytes,omitempty"`
	Packets int `json:"packets,omitempty"`

	codecHandler CodecHandler
}

func NewReceiver(media *Media, codec *Codec) *Receiver {
	r := &Receiver{
		Node:  Node{id: NewID(), Codec: codec},
		Media: media,
	}

	r.SetOwner(r)

	r.Input = func(packet *Packet) {
		r.Bytes += len(packet.Payload)
		r.Packets++

		if r.codecHandler != nil {
			fmt.Printf("[RECEIVER] Receiver %d received packet, sequence=%d, timestampe=%d, len=%d\n",
				r.id, packet.Header.SequenceNumber, packet.Header.Timestamp, len(packet.Payload))

			r.codecHandler.ProcessPacket(packet)
		}

		for _, child := range r.childs {
			child.Input(packet)
		}
	}
	return r
}

func (r *Receiver) SetupGOP() {
	if !r.Codec.IsVideo() {
		fmt.Printf("[RECEIVER] Receiver %d is not video codec, skipping GOP setup\n", r.id)
		return
	}

	if r.codecHandler != nil {
		fmt.Printf("[RECEIVER] Receiver %d already has codec handler, skipping GOP setup\n", r.id)
		return
	}

	fmt.Printf("[RECEIVER] Receiver %d setting up GOP for codec %s\n", r.id, r.Codec.Name)

	if handler := CreateCodecHandler(r.Codec); handler != nil {
		r.codecHandler = handler
	}
}

// Deprecated: should be removed
func (r *Receiver) WriteRTP(packet *rtp.Packet) {
	r.Input(packet)
}

// Deprecated: should be removed
func (r *Receiver) Senders() []*Sender {
	if len(r.childs) > 0 {
		return []*Sender{{}}
	} else {
		return nil
	}
}

// Deprecated: should be removed
func (r *Receiver) Replace(target *Receiver) {
	MoveNode(&target.Node, &r.Node)
}

func (r *Receiver) Close() {
	if r.codecHandler != nil {
		r.codecHandler.ClearCache()
	}
	r.Node.Close()
}

type Sender struct {
	Node

	InputCache HandlerFunc

	// Deprecated:
	Media *Media `json:"-"`
	// Deprecated:
	Handler HandlerFunc `json:"-"`

	Bytes   int `json:"bytes,omitempty"`
	Packets int `json:"packets,omitempty"`
	Drops   int `json:"drops,omitempty"`

	buf  chan *Packet
	done chan struct{}

	started         bool
	waitingForCache bool
	liveQueue       chan *Packet
}

func NewSender(media *Media, codec *Codec) *Sender {
	var bufSize uint16

	if GetKind(codec.Name) == KindVideo {
		if codec.IsRTP() {
			// in my tests 40Mbit/s 4K-video can generate up to 1500 items
			// for the h264.RTPDepay => RTPPay queue
			bufSize = 4096
		} else {
			bufSize = 64
		}
	} else {
		bufSize = 128
	}

	buf := make(chan *Packet, bufSize)
	s := &Sender{
		Node:            Node{id: NewID(), Codec: codec},
		Media:           media,
		buf:             buf,
		liveQueue:       make(chan *Packet, 256),
		waitingForCache: true,
	}

	fmt.Printf("[SENDER] New Sender %d, codec=%s, sender codec=%s, media kind=%s\n",
		s.id, codec.Name, s.Codec.Name, media.Kind)

	s.SetOwner(s)

	s.Input = func(packet *Packet) {
		if !s.started && s.Codec.IsVideo() {
			fmt.Printf("[SENDER] Sender %d not started, ignoring packet: sequence=%d, timestamp=%d, len=%d\n",
				s.id, packet.Header.SequenceNumber, packet.Header.Timestamp, len(packet.Payload))
			return
		}

		if s.Codec.IsVideo() {
			if s.waitingForCache {
				select {
				case s.liveQueue <- packet:
					fmt.Printf("[SENDER] Sender %d waiting for cache, queueing packet: sequence=%d, timestamp=%d, len=%d, queue=%d\n",
						s.id, packet.Header.SequenceNumber, packet.Header.Timestamp, len(packet.Payload), len(s.liveQueue))
				default:
					fmt.Printf("[SENDER] Sender %d live queue is full, dropping packet: sequence=%d, timestamp=%d, len=%d, queue=%d\n",
						s.id, packet.Header.SequenceNumber, packet.Header.Timestamp, len(packet.Payload), len(s.liveQueue))
				}
				return
			}
		}

		fmt.Printf("[SENDER] Sender %d processing packet: sequence=%d, timestamp=%d, len=%d\n",
			s.id, packet.Header.SequenceNumber, packet.Header.Timestamp, len(packet.Payload))

		s.processPacket(packet)
	}

	s.InputCache = func(packet *Packet) {
		s.processPacket(packet)
	}
	s.Output = func(packet *Packet) {
		s.Handler(packet)
	}
	return s
}

// Deprecated: should be removed
func (s *Sender) HandleRTP(parent *Receiver) {
	s.WithParent(parent)
	s.Start()
}

// Deprecated: should be removed
func (s *Sender) Bind(parent *Receiver) {
	s.WithParent(parent)
}

func (s *Sender) WithParent(parent *Receiver) *Sender {
	s.Node.WithParent(&parent.Node)
	return s
}

func (s *Sender) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.buf == nil || s.done != nil {
		return
	}
	s.done = make(chan struct{})

	// pass buf directly so that it's impossible for buf to be nil
	go func(buf chan *Packet) {
		for packet := range buf {
			s.Output(packet)
		}
		close(s.done)
	}(s.buf)

	s.started = true

	if !s.Codec.IsVideo() {
		fmt.Printf("[SENDER] Sender %d is not video codec, skipping cache processing\n", s.id)
		s.waitingForCache = false
		return
	}

	go func() {
		if receiver, ok := s.parent.owner.(*Receiver); ok {
			if receiver.codecHandler != nil {
				nextTimestamp := receiver.codecHandler.SendCacheTo(s, 100)
				receiver.codecHandler.SendQueueTo(s, 100, nextTimestamp)
			}
		}

		fmt.Printf("[SENDER] Sender %d finished processing cache, starting to process live queue\n", s.id)

		s.waitingForCache = false
	}()
}

func (s *Sender) Wait() {
	if done := s.done; done != nil {
		<-done
	}
}

func (s *Sender) State() string {
	if s.buf == nil {
		return "closed"
	}
	if s.done == nil {
		return "new"
	}
	return "connected"
}

func (s *Sender) Close() {
	// close buffer if exists
	s.mu.Lock()
	if s.buf != nil {
		close(s.buf) // exit from for range loop
		s.buf = nil  // prevent writing to closed chan
	}
	if s.liveQueue != nil {
		close(s.liveQueue)
		s.liveQueue = nil
	}
	s.mu.Unlock()

	s.Node.Close()
}

func (r *Receiver) MarshalJSON() ([]byte, error) {
	v := struct {
		ID      uint32   `json:"id"`
		Codec   *Codec   `json:"codec"`
		Childs  []uint32 `json:"childs,omitempty"`
		Bytes   int      `json:"bytes,omitempty"`
		Packets int      `json:"packets,omitempty"`
	}{
		ID:      r.Node.id,
		Codec:   r.Node.Codec,
		Bytes:   r.Bytes,
		Packets: r.Packets,
	}
	for _, child := range r.childs {
		v.Childs = append(v.Childs, child.id)
	}
	return json.Marshal(v)
}

func (s *Sender) MarshalJSON() ([]byte, error) {
	v := struct {
		ID      uint32 `json:"id"`
		Codec   *Codec `json:"codec"`
		Parent  uint32 `json:"parent,omitempty"`
		Bytes   int    `json:"bytes,omitempty"`
		Packets int    `json:"packets,omitempty"`
		Drops   int    `json:"drops,omitempty"`
	}{
		ID:      s.Node.id,
		Codec:   s.Node.Codec,
		Bytes:   s.Bytes,
		Packets: s.Packets,
		Drops:   s.Drops,
	}
	if s.parent != nil {
		v.Parent = s.parent.id
	}
	return json.Marshal(v)
}

func (s *Sender) processPacket(packet *Packet) {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case s.buf <- packet:
		s.Bytes += len(packet.Payload)
		s.Packets++
	default:
		s.Drops++
	}
}
