package hls

import (
	"fmt"
	"sync"
	"time"
)

type Session struct {
	cons     Consumer
	template string
	init     []byte
	segment0 []byte
	buffer   []byte
	seq      int
	alive    *time.Timer
	mu       sync.Mutex
}

func (s *Session) Playlist() string {
	return fmt.Sprintf(s.template, s.seq, s.seq, s.seq+1)
}

func (s *Session) Segment() (segment []byte) {
	for i := 0; i < 20 && segment == nil; i++ {
		if i > 0 {
			time.Sleep(50 * time.Millisecond)
		}

		s.mu.Lock()
		if len(s.buffer) > 0 {
			segment = s.buffer
			// for TS important to start new segment with init
			s.buffer = s.init
			s.seq++
		}
		s.mu.Unlock()
	}

	return
}
