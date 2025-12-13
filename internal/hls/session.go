package hls

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
)

// MaxBufferSize limits the HLS segment buffer to prevent memory leaks
// when clients don't fetch segments. 16MB should be enough for ~30 seconds
// of high-quality video at typical bitrates.
const MaxBufferSize = 16 * 1024 * 1024

type Session struct {
	cons     core.Consumer
	id       string
	template string
	init     []byte
	buffer   []byte
	seq      int
	alive    *time.Timer
	mu       sync.Mutex
	dropped  int // count of dropped writes due to buffer overflow
}

func NewSession(cons core.Consumer) *Session {
	s := &Session{
		id:   core.RandString(8, 62),
		cons: cons,
	}

	// two segments important for Chromecast
	if _, ok := cons.(*mp4.Consumer); ok {
		s.template = `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:%d
#EXT-X-MAP:URI="init.mp4?id=` + s.id + `"
#EXTINF:0.500,
segment.m4s?id=` + s.id + `&n=%d
#EXTINF:0.500,
segment.m4s?id=` + s.id + `&n=%d`
	} else {
		s.template = `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:%d
#EXTINF:0.500,
segment.ts?id=` + s.id + `&n=%d
#EXTINF:0.500,
segment.ts?id=` + s.id + `&n=%d`
	}

	return s
}

func (s *Session) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.init == nil {
		s.init = p
		return len(p), nil
	}

	// Check if adding this data would exceed the buffer limit
	if len(s.buffer)+len(p) > MaxBufferSize {
		// Buffer is full - drop old data to make room
		// This prevents unbounded memory growth when clients don't consume segments
		s.dropped++

		// If buffer is way too big, reset it entirely
		if len(s.buffer) > MaxBufferSize {
			s.buffer = nil
		} else {
			// Trim the beginning of the buffer to make room
			trimSize := len(p)
			if trimSize > len(s.buffer) {
				trimSize = len(s.buffer)
			}
			s.buffer = s.buffer[trimSize:]
		}
	}

	s.buffer = append(s.buffer, p...)
	return len(p), nil
}

func (s *Session) Run() {
	_, _ = s.cons.(io.WriterTo).WriteTo(s)
}

func (s *Session) Main() []byte {
	type withCodecs interface {
		Codecs() []*core.Codec
	}

	codecs := mp4.MimeCodecs(s.cons.(withCodecs).Codecs())
	codecs = strings.Replace(codecs, mp4.MimeFlac, "fLaC", 1)

	// bandwidth important for Safari, codecs useful for smooth playback
	return []byte(`#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=192000,CODECS="` + codecs + `"
hls/playlist.m3u8?id=` + s.id)
}

func (s *Session) Playlist() []byte {
	return []byte(fmt.Sprintf(s.template, s.seq, s.seq, s.seq+1))
}

func (s *Session) Init() (init []byte) {
	for i := 0; i < 60 && init == nil; i++ {
		if i > 0 {
			time.Sleep(50 * time.Millisecond)
		}

		s.mu.Lock()
		// return init only when have some buffer
		if len(s.buffer) > 0 {
			init = s.init
		}
		s.mu.Unlock()
	}

	return
}

func (s *Session) Segment() (segment []byte) {
	for i := 0; i < 60 && segment == nil; i++ {
		if i > 0 {
			time.Sleep(50 * time.Millisecond)
		}

		s.mu.Lock()
		if len(s.buffer) > 0 {
			segment = s.buffer
			if _, ok := s.cons.(*mp4.Consumer); ok {
				s.buffer = nil
			} else {
				// for TS important to start new segment with init
				s.buffer = s.init
			}
			s.seq++
		}
		s.mu.Unlock()
	}

	return
}
