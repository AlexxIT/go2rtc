package mjpeg

import (
	"io"
	"net/http"
	"strconv"
)

func NewWriter(w io.Writer) io.Writer {
	h := w.(http.ResponseWriter).Header()
	h.Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	return &writer{wr: w, buf: []byte(header)}
}

const header = "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: "

type writer struct {
	wr  io.Writer
	buf []byte
}

func (w *writer) Write(p []byte) (n int, err error) {
	w.buf = w.buf[:len(header)]
	w.buf = append(w.buf, strconv.Itoa(len(p))...)
	w.buf = append(w.buf, "\r\n\r\n"...)
	w.buf = append(w.buf, p...)
	w.buf = append(w.buf, "\r\n"...)

	// Chrome bug: mjpeg image always shows the second to last image
	// https://bugs.chromium.org/p/chromium/issues/detail?id=527446
	if _, err = w.wr.Write(w.buf); err != nil {
		return 0, err
	}

	w.wr.(http.Flusher).Flush()

	return len(p), nil
}
