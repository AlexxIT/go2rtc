package core

import (
	"bytes"
	"io"
	"net/http"
	"sync"
)

// WriteBuffer by defaul Write(s) to bytes.Buffer.
// But after WriteTo to new io.Writer - calls Reset.
// Reset will flush current buffer data to new writer and starts to Write to new io.Writer
// WriteTo will be locked until Write fails or Close will be called.
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
	defer w.mu.Unlock()

	if w.err != nil {
		return 0, w.err
	}
	if w.Writer == nil {
		w.err = io.ErrClosedPipe
		w.done()
		return 0, w.err
	}
	n, err = w.Writer.Write(p)
	if err != nil {
		w.err = err
		w.done()
	} else if f, ok := w.Writer.(http.Flusher); ok {
		f.Flush()
	}
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
	defer w.mu.Unlock()
	w.add()
	if buf, ok := w.Writer.(*bytes.Buffer); ok && buf.Len() != 0 {
		if _, err := io.Copy(wr, buf); err != nil {
			w.err = err
			w.done()
		}
	}
	w.Writer = wr
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

// OnceBuffer will catch only first message
type OnceBuffer struct {
	buf []byte
}

func (o *OnceBuffer) Write(p []byte) (n int, err error) {
	if o.buf == nil {
		o.buf = p
	}
	return 0, io.EOF
}

func (o *OnceBuffer) WriteTo(w io.Writer) (n int64, err error) {
	return io.Copy(w, bytes.NewReader(o.buf))
}

func (o *OnceBuffer) Buffer() []byte {
	return o.buf
}

func (o *OnceBuffer) Len() int {
	return len(o.buf)
}
