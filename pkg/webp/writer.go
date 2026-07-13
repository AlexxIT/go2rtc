package webp

import (
	"io"
	"net/http"
	"strconv"
)

const header = "--frame\r\nContent-Type: image/webp\r\nContent-Length: "

// Writer writes multipart WebP frames to an HTTP response.
type Writer struct {
	wr  io.Writer
	buf []byte
}

// NewWriter creates a Writer that sets the multipart Content-Type header.
func NewWriter(w io.Writer) *Writer {
	h := w.(http.ResponseWriter).Header()
	h.Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	return &Writer{wr: w, buf: []byte(header)}
}

func (w *Writer) Write(p []byte) (n int, err error) {
	w.buf = w.buf[:len(header)]
	w.buf = append(w.buf, strconv.Itoa(len(p))...)
	w.buf = append(w.buf, "\r\n\r\n"...)
	w.buf = append(w.buf, p...)
	w.buf = append(w.buf, "\r\n"...)

	if _, err = w.wr.Write(w.buf); err != nil {
		return 0, err
	}

	w.wr.(http.Flusher).Flush()

	return len(p), nil
}
