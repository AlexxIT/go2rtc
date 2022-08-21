package streamer

import (
	"fmt"
	"github.com/pion/rtp"
	"sync"
)

type WriterFunc func(packet *rtp.Packet) error
type WrapperFunc func(push WriterFunc) WriterFunc

type Track struct {
	Codec     *Codec
	Direction string
	Sink      map[*Track]WriterFunc
	mx        sync.Mutex
}

func (t *Track) String() string {
	s := t.Codec.String()
	s += fmt.Sprintf(", sinks=%d", len(t.Sink))
	return s
}

func (t *Track) WriteRTP(p *rtp.Packet) error {
	t.mx.Lock()
	for _, f := range t.Sink {
		_ = f(p)
	}
	t.mx.Unlock()
	return nil
}

func (t *Track) Bind(w WriterFunc) *Track {
	if t.Sink == nil {
		t.Sink = map[*Track]WriterFunc{}
	}

	clone := &Track{
		Codec: t.Codec, Direction: t.Direction, Sink: t.Sink,
	}
	t.mx.Lock()
	t.Sink[clone] = w
	t.mx.Unlock()
	return clone
}

func (t *Track) Unbind() {
	t.mx.Lock()
	delete(t.Sink, t)
	t.mx.Unlock()
}
