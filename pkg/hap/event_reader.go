package hap

import (
	"io"
	"os"
	"time"
)

type EventReader struct {
	r    io.Reader
	ch   chan []byte
	err  error
	left []byte
}

func NewEventReader(r io.Reader) *EventReader {
	e := &EventReader{r: r, ch: make(chan []byte, 1)}
	go e.background()
	return e
}

func (e *EventReader) background() {
	b := make([]byte, 32*1024)
	for {
		n, err := e.r.Read(b)
		if err != nil {
			e.err = err
			return
		}

		if n >= 6 && string(b[:6]) == "EVENT " {
			panic("TODO")
		}

		// copy because will be overwriten
		buf := make([]byte, n)
		copy(buf, b)
		e.ch <- buf
	}
}

func (e *EventReader) Read(p []byte) (n int, err error) {
	if e.err != nil {
		return 0, e.err
	}

	// if something left after previous reading
	if e.left != nil {
		// if still something left
		if n = copy(p, e.left); n < len(e.left) {
			e.left = e.left[n:]
		} else {
			e.left = nil
		}
		return
	}

	select {
	case <-time.After(time.Second * 5):
		return 0, os.ErrDeadlineExceeded
	case b := <-e.ch:
		if n = copy(p, b); n < len(b) {
			e.left = b[n:]
		}
	}

	return
}
