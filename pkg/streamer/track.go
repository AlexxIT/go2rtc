package streamer

import (
	"encoding/json"
	"fmt"
	"github.com/pion/rtp"
	"sync"
)

type WriterFunc func(packet *rtp.Packet) error
type WrapperFunc func(push WriterFunc) WriterFunc

type Track struct {
	Codec     *Codec
	Direction string
	sink      map[*Track]WriterFunc
	sinkMu    *sync.RWMutex
}

func NewTrack(codec *Codec, direction string) *Track {
	return &Track{Codec: codec, Direction: direction, sinkMu: new(sync.RWMutex)}
}

func NewTrack2(media *Media, codec *Codec) *Track {
	if codec == nil {
		codec = media.Codecs[0]
	}
	return &Track{Codec: codec, Direction: media.Direction, sinkMu: new(sync.RWMutex)}
}

func (t *Track) String() string {
	s := t.Codec.String()
	if t.sinkMu.TryRLock() {
		s += fmt.Sprintf(", sinks=%d", len(t.sink))
		t.sinkMu.RUnlock()
	} else {
		s += fmt.Sprintf(", sinks=?")
	}
	return s
}

func (t *Track) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *Track) WriteRTP(p *rtp.Packet) error {
	t.sinkMu.RLock()
	for _, f := range t.sink {
		_ = f(p)
	}
	t.sinkMu.RUnlock()
	return nil
}

func (t *Track) Bind(w WriterFunc) *Track {
	t.sinkMu.Lock()

	if t.sink == nil {
		t.sink = map[*Track]WriterFunc{}
	}

	clone := *t
	t.sink[&clone] = w

	t.sinkMu.Unlock()

	return &clone
}

func (t *Track) Unbind() {
	t.sinkMu.Lock()
	delete(t.sink, t)
	t.sinkMu.Unlock()
}

func (t *Track) GetSink(from *Track) {
	t.sinkMu.Lock()
	t.sink = from.sink
	t.sinkMu.Unlock()
}

func (t *Track) HasSink() bool {
	t.sinkMu.RLock()
	defer t.sinkMu.RUnlock()
	return len(t.sink) > 0
}
