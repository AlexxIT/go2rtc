package core

import (
	"errors"
	"io"
)

const ProbeSize = 1024 * 1024 // 1MB

const (
	BufferDisable       = 0
	BufferDrainAndClear = -1
)

// ReadBuffer support buffering and Seek over buffer
// positive BufferSize will enable buffering mode
// Seek to negative offset will clear buffer
// Seek with a positive BufferSize will continue buffering after the last read from the buffer
// Seek with a negative BufferSize will clear buffer after the last read from the buffer
// Read more than BufferSize will raise error
type ReadBuffer struct {
	io.Reader

	BufferSize int

	buf []byte
	pos int
}

func NewReadBuffer(rd io.Reader) *ReadBuffer {
	if rs, ok := rd.(*ReadBuffer); ok {
		return rs
	}
	return &ReadBuffer{Reader: rd}
}

func (r *ReadBuffer) Read(p []byte) (n int, err error) {
	// with zero buffer - read as usual
	if r.BufferSize == BufferDisable {
		return r.Reader.Read(p)
	}

	// if buffer not empty - read from it
	if r.pos < len(r.buf) {
		n = copy(p, r.buf[r.pos:])
		r.pos += n
		return
	}

	// with negative buffer - empty it and read as usual
	if r.BufferSize < 0 {
		r.BufferSize = BufferDisable
		r.buf = nil
		r.pos = 0

		return r.Reader.Read(p)
	}

	n, err = r.Reader.Read(p)
	if len(r.buf)+n > r.BufferSize {
		return 0, errors.New("probe reader overflow")
	}
	r.buf = append(r.buf, p[:n]...)
	r.pos += n
	return
}

func (r *ReadBuffer) Close() error {
	if closer, ok := r.Reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (r *ReadBuffer) Seek(offset int64, whence int) (int64, error) {
	var pos int
	switch whence {
	case io.SeekStart:
		pos = int(offset)
	case io.SeekCurrent:
		pos = r.pos + int(offset)
	case io.SeekEnd:
		pos = len(r.buf) + int(offset)
	}

	// negative offset - empty buffer
	if pos < 0 {
		r.buf = nil
		r.pos = 0
	} else if pos >= len(r.buf) {
		r.pos = len(r.buf)
	} else {
		r.pos = pos
	}

	return int64(r.pos), nil
}

func (r *ReadBuffer) Peek(n int) ([]byte, error) {
	r.BufferSize = n
	b := make([]byte, n)
	if _, err := io.ReadAtLeast(r, b, n); err != nil {
		return nil, err
	}
	r.Reset()
	return b, nil
}

func (r *ReadBuffer) Reset() {
	r.BufferSize = BufferDrainAndClear
	r.pos = 0
}
