package miss

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
)

// LensPair belongs to one explicitly configured pair of Xiaomi streams.
type LensPair struct {
	mu      sync.Mutex
	session *lensSession
}

type lensSession struct {
	camera *Producer
	opened [2]bool
	start  sync.Once
	done   chan struct{}
	err    error
}

type lens struct {
	*Producer
	pair    *LensPair
	session *lensSession
	done    chan struct{}
	channel int
}

// Open signs and dials the primary URL only when the first view needs the camera.
func (p *LensPair) Open(secondary bool, cameraURL func() (string, error)) (core.Producer, error) {
	channel := 0
	if secondary {
		channel = 1
	}
	v, err := p.open(channel, func() (*Producer, error) {
		rawURL, err := cameraURL()
		if err != nil {
			return nil, err
		}
		return dial(rawURL, true)
	})
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (p *LensPair) open(channel int, connect func() (*Producer, error)) (*lens, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.session
	if s == nil {
		camera, err := connect()
		if err != nil {
			return nil, err
		}
		// The two native talkback senders share the transport sequence number.
		camera.client.Conn = &lensTransport{Conn: camera.client.Conn}
		s = &lensSession{camera: camera, done: make(chan struct{})}
		// Each view needs its own receiver even for the shared microphone:
		// streams reconnects a receiver by moving all its subscribers.
		camera.lenses = new(lensOutputs)
		for i := range camera.lenses {
			out := &camera.lenses[i]
			out.Medias = append([]*core.Media{camera.Medias[i]}, camera.Medias[2:]...)
			for _, m := range out.Medias {
				if m.Direction == core.DirectionRecvonly {
					_, _ = out.GetTrack(m, m.Codecs[0])
				}
			}
		}
		p.session = s
	} else if s.opened[channel] {
		return nil, errors.New("xiaomi: lens already in use; subscribe to its configured stream")
	}
	s.opened[channel] = true
	out := s.camera.lenses[channel].Connection
	out.ID, out.FormatName = core.NewID(), "xiaomi/miss"
	out.Protocol, out.RemoteAddr, out.UserAgent = s.camera.Protocol, s.camera.RemoteAddr, s.camera.UserAgent
	// Native AddTrack writes to the same camera. Each view owns its senders.
	return &lens{Producer: &Producer{Connection: out, client: s.camera.client}, pair: p, session: s, channel: channel, done: make(chan struct{})}, nil
}

func (v *lens) Start() error {
	s := v.session
	v.pair.mu.Lock()
	select {
	case <-v.done:
		v.pair.mu.Unlock()
		return nil
	default:
	}
	// The stream has now attached its subscriber. A late view needs its own keyframe.
	s.camera.lenses[v.channel].state.CompareAndSwap(lensIdle, lensWaitKey)
	v.pair.mu.Unlock()
	s.start.Do(func() {
		go func() {
			err := s.camera.Start()
			v.pair.mu.Lock()
			if v.pair.session == s {
				v.pair.session = nil
			}
			s.err = err
			close(s.done)
			v.pair.mu.Unlock()
		}()
	})
	select {
	case <-s.done:
		return s.err
	case <-v.done:
		return nil
	}
}

type lensTransport struct {
	Conn
	mu        sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

func (t *lensTransport) Close() error {
	t.closeOnce.Do(func() { t.closeErr = t.Conn.Close() })
	return t.closeErr
}

func (t *lensTransport) WritePacket(header, payload []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Conn.WritePacket(header, payload)
}

func (v *lens) Stop() error {
	v.pair.mu.Lock()
	select {
	case <-v.done:
		v.pair.mu.Unlock()
		return nil
	default:
		close(v.done)
	}
	for _, sender := range v.Senders {
		sender.Close()
	}
	s := v.session
	s.opened[v.channel] = false
	s.camera.lenses[v.channel].state.Store(lensIdle)
	if s.opened[0] || s.opened[1] {
		v.pair.mu.Unlock()
		return nil
	}
	var err error
	if v.pair.session == s {
		// Release the slot before a new subscriber can open another session.
		v.pair.session = nil
		_ = s.camera.client.StopMedia()
		err = s.camera.client.Close()
	}
	v.pair.mu.Unlock()
	s.start.Do(func() { close(s.done) }) // a probe can be stopped without Start
	<-s.done
	for i := range s.camera.lenses {
		_ = s.camera.lenses[i].Connection.Stop()
	}
	return err
}

const (
	lensIdle = iota
	lensWaitKey
	lensStreaming
)

type lensOutputs [2]struct {
	core.Connection
	state atomic.Uint32
}

func (views *lensOutputs) write(pkt *Packet, codec string, packet *core.Packet) {
	switch pkt.CodecID {
	case codecH264, codecH265:
		v := &views[pkt.videoChannel()]
		r := v.Receivers[0]
		if r.Codec.Name != codec {
			return
		}
		switch v.state.Load() {
		case lensIdle:
			return
		case lensWaitKey:
			if (pkt.CodecID == codecH264 && !h264.IsKeyframe(packet.Payload)) || (pkt.CodecID == codecH265 && !h265.IsKeyframe(packet.Payload)) {
				return
			}
			if !v.state.CompareAndSwap(lensWaitKey, lensStreaming) {
				return
			}
		}
		r.WriteRTP(packet)
	case codecPCMA, codecOPUS:
		var first *core.Receiver
		for i := range views {
			v := &views[i]
			if v.state.Load() == lensIdle || len(v.Receivers) < 2 || v.Receivers[1].Codec.Name != codec {
				continue
			}
			if first == nil {
				first = v.Receivers[1]
				continue
			}
			// Consumers can change the RTP marker. Share only the encoded payload.
			other := *packet
			v.Receivers[1].WriteRTP(&other)
		}
		if first != nil {
			first.WriteRTP(packet)
		}
	}
}
