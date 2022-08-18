package streamer

import (
	"fmt"
	"github.com/pion/rtp"
)

type WriterFunc func(packet *rtp.Packet) error
type WrapperFunc func(push WriterFunc) WriterFunc

type Track struct {
	Codec     *Codec
	Direction string
	Sink      map[*Track]WriterFunc
}

func (t *Track) String() string {
	s := t.Codec.String()
	s += fmt.Sprintf(", sinks=%d", len(t.Sink))
	return s
}

func (t *Track) WriteRTP(p *rtp.Packet) error {
	for _, f := range t.Sink {
		_ = f(p)
	}
	return nil
}

func (t *Track) Bind(w WriterFunc) *Track {
	if t.Sink == nil {
		t.Sink = map[*Track]WriterFunc{}
	}

	clone := &Track{
		Codec: t.Codec, Direction: t.Direction, Sink: t.Sink,
	}
	t.Sink[clone] = w
	return clone
}

func (t *Track) Unbind() {
	delete(t.Sink, t)
}
