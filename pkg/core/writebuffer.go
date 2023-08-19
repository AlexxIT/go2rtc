package core

import (
	"bytes"
	"io"
	"sync"
)

type WriteBuffer struct {
	io.Writer
	err   error
	mu    sync.Mutex
	wg    sync.WaitGroup
	state byte
}

func NewWriteBuffer(wr io.Writer) *WriteBuffer {
	if wr == nil {
		wr = bytes.NewBuffer(nil)
	}
	return &WriteBuffer{Writer: wr}
}

func (w *WriteBuffer) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	if w.err != nil {
		err = w.err
	} else if n, err = w.Writer.Write(p); err != nil {
		w.err = err
		w.done()
	}
	w.mu.Unlock()
	return
}

func (w *WriteBuffer) WriteTo(wr io.Writer) (n int64, err error) {
	w.Reset(wr)
	w.wg.Wait()
	return 0, w.err // TODO: fix counter
}

func (w *WriteBuffer) Close() error {
	if closer, ok := w.Writer.(io.Closer); ok {
		return closer.Close()
	}
	w.mu.Lock()
	w.done()
	w.mu.Unlock()
	return nil
}

func (w *WriteBuffer) Reset(wr io.Writer) {
	w.mu.Lock()
	w.add()
	if buf, ok := wr.(*bytes.Buffer); ok {
		if _, err := io.Copy(wr, buf); err != nil {
			w.err = err
			w.done()
		}
	}
	w.Writer = wr
	w.mu.Unlock()
}

const (
	none = iota
	start
	end
)

func (w *WriteBuffer) add() {
	if w.state == none {
		w.state = start
		w.wg.Add(1)
	}
}

func (w *WriteBuffer) done() {
	if w.state == start {
		w.state = end
		w.wg.Done()
	}
}
