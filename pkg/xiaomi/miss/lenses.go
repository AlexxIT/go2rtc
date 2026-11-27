package miss

import (
	"encoding/json"
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
	users  int
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
	state   atomic.Uint32
	recv    atomic.Int64
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
		camera.lenses = new(lensOutputs)
		p.session = s
	}
	s.users++
	camera := s.camera
	out := core.Connection{
		ID: core.NewID(), FormatName: camera.FormatName, Protocol: camera.Protocol,
		RemoteAddr: camera.RemoteAddr, UserAgent: camera.UserAgent,
		Medias: append([]*core.Media{camera.Medias[channel]}, camera.Medias[2:]...),
	}
	for _, m := range out.Medias {
		if m.Direction == core.DirectionRecvonly {
			_, _ = out.GetTrack(m, m.Codecs[0])
		}
	}
	return &lens{Producer: &Producer{Connection: out, client: camera.client}, pair: p, session: s, channel: channel, done: make(chan struct{})}, nil
}

// A playback-only handle can coexist with the named stream's forward reader.
func (v *lens) GetTrack(_ *core.Media, codec *core.Codec) (*core.Receiver, error) {
	v.pair.mu.Lock()
	defer v.pair.mu.Unlock()
	select {
	case <-v.done:
		return nil, core.ErrCantGetTrack
	default:
	}
	for _, r := range v.Receivers {
		if r.Codec == codec {
			slot := &v.session.camera.lenses[v.channel]
			if owner := slot.Load(); owner != nil && owner != v {
				return nil, errors.New("xiaomi: lens already in use; subscribe to its configured stream")
			}
			slot.Store(v)
			return r, nil
		}
	}
	return nil, core.ErrCantGetTrack
}

func (v *lens) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		*core.Connection
		Recv int64 `json:"bytes_recv,omitempty"`
	}{&v.Connection, v.recv.Load()})
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
	v.state.CompareAndSwap(lensIdle, lensWaitKey)
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
	s.camera.lenses[v.channel].CompareAndSwap(v, nil)
	s.users--
	if s.users != 0 {
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
	return err
}

const (
	lensIdle = iota
	lensWaitKey
	lensStreaming
)

type lensOutputs [2]atomic.Pointer[lens]

func (views *lensOutputs) write(pkt *Packet, codec string, packet *core.Packet) {
	switch pkt.CodecID {
	case codecH264, codecH265:
		v := views[pkt.videoChannel()].Load()
		if v == nil {
			return
		}
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
		v.recv.Add(int64(len(packet.Payload)))
		r.WriteRTP(packet)
	case codecPCMA, codecOPUS:
		var first *core.Receiver
		for i := range views {
			v := views[i].Load()
			if v == nil || v.state.Load() == lensIdle || len(v.Receivers) < 2 || v.Receivers[1].Codec.Name != codec {
				continue
			}
			v.recv.Add(int64(len(packet.Payload)))
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
