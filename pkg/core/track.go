package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pion/rtp"
	"strconv"
	"sync"
)

var ErrCantGetTrack = errors.New("can't get track")

type Receiver struct {
	Codec *Codec
	Media *Media

	ID byte // Channel for RTSP, PayloadType for MPEG-TS

	senders map[*Sender]chan *rtp.Packet
	mu      sync.RWMutex
	bytes   int
}

func NewReceiver(media *Media, codec *Codec) *Receiver {
	Assert(codec != nil)
	return &Receiver{Codec: codec, Media: media}
}

// WriteRTP - fast and non blocking write to all readers buffers
func (t *Receiver) WriteRTP(packet *rtp.Packet) {
	t.mu.Lock()
	t.bytes += len(packet.Payload)
	for sender, buffer := range t.senders {
		select {
		case buffer <- packet:
		default:
			sender.overflow++
		}
	}
	t.mu.Unlock()
}

func (t *Receiver) Senders() (senders []*Sender) {
	t.mu.RLock()
	for sender := range t.senders {
		senders = append(senders, sender)
	}
	t.mu.RUnlock()
	return
}

func (t *Receiver) Close() {
	t.mu.Lock()
	// close all sender channel buffers and erase senders list
	for _, buffer := range t.senders {
		close(buffer)
	}
	t.senders = nil
	t.mu.Unlock()
}

func (t *Receiver) Replace(target *Receiver) {
	// move this receiver senders to new receiver
	t.mu.Lock()
	senders := t.senders
	t.mu.Unlock()

	target.mu.Lock()
	target.senders = senders
	target.mu.Unlock()
}

func (t *Receiver) String() string {
	s := t.Codec.String() + ", bytes=" + strconv.Itoa(t.bytes)
	t.mu.RLock()
	s += fmt.Sprintf(", senders=%d", len(t.senders))
	t.mu.RUnlock()
	return s
}

func (t *Receiver) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

type Sender struct {
	Codec *Codec
	Media *Media

	Handler HandlerFunc

	receivers []*Receiver
	mu        sync.RWMutex
	bytes     int

	overflow int
}

func NewSender(media *Media, codec *Codec) *Sender {
	return &Sender{Codec: codec, Media: media}
}

// HandlerFunc like http.HandlerFunc
type HandlerFunc func(packet *rtp.Packet)

func (s *Sender) HandleRTP(track *Receiver) {
	bufferSize := 100

	if GetKind(track.Codec.Name) == KindVideo {
		if track.Codec.IsRTP() {
			// H.264 2560x1440 4096kbs can have 700+ packets between 25 frames
			// H.265 5120x1440 can have 700+ packets between two keyframes
			bufferSize = 1000
		} else {
			bufferSize = 50
		}
	}

	buffer := make(chan *rtp.Packet, bufferSize)

	track.mu.Lock()
	if track.senders == nil {
		track.senders = map[*Sender]chan *rtp.Packet{}
	}
	track.senders[s] = buffer
	track.mu.Unlock()
	s.mu.Lock()
	s.receivers = append(s.receivers, track)
	s.mu.Unlock()

	go func() {
		// read packets from buffer channel until it will be closed
		for packet := range buffer {
			s.mu.Lock()
			s.bytes += len(packet.Payload)
			s.mu.Unlock()
			s.Handler(packet)
		}

		// remove current receiver from list
		// it can only happen when receiver close buffer channel
		s.mu.Lock()
		for i, receiver := range s.receivers {
			if receiver == track {
				s.receivers = append(s.receivers[:i], s.receivers[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
	}()
}

func (s *Sender) Close() {
	s.mu.Lock()
	// remove this sender from all receivers list
	for _, receiver := range s.receivers {
		receiver.mu.Lock()
		if buffer := receiver.senders[s]; buffer != nil {
			// remove channel from list
			delete(receiver.senders, s)
			// close channel
			close(buffer)
		}
		receiver.mu.Unlock()
	}
	s.receivers = nil
	s.mu.Unlock()
}

func (s *Sender) String() string {
	info := s.Codec.String() + ", bytes=" + strconv.Itoa(s.bytes)
	s.mu.RLock()
	info += ", receivers=" + strconv.Itoa(len(s.receivers))
	s.mu.RUnlock()
	if s.overflow > 0 {
		info += ", overflow=" + strconv.Itoa(s.overflow)
	}
	return info
}

func (s *Sender) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}
