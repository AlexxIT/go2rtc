package tapo

import (
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/textproto"
	"strconv"
	"testing"
	"time"
)

// runHandle starts Client.Handle on one end of a net.Pipe, lets write feed the
// other end, and returns the error from Handle (or fails on timeout).
func runHandle(t *testing.T, write func(w *multipart.Writer)) error {
	t.Helper()

	c1, c2 := net.Pipe()
	defer c2.Close()

	c := &Client{conn1: c1, decrypt: func(b []byte) []byte { return b }}
	defer c.Close()

	done := make(chan error, 1)
	go func() { done <- c.Handle() }()

	go func() {
		w := multipart.NewWriter(c2)
		_ = w.SetBoundary("--device-stream-boundary--")
		write(w)
		_ = w.Close()
		_ = c2.Close()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("Handle did not return in time (busy loop?)")
		return nil
	}
}

func addPart(t *testing.T, w *multipart.Writer, length int, body []byte) {
	t.Helper()

	pw, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type":   {"video/mp2t"},
		"Content-Length": {strconv.Itoa(length)},
	})
	if err != nil {
		return
	}
	_, _ = pw.Write(body)
}

// A part with more bytes than its Content-Length used to make Handle spin.
func TestHandleOversizedPartDoesNotSpin(t *testing.T) {
	err := runHandle(t, func(w *multipart.Writer) {
		addPart(t, w, 0, []byte("abcd"))
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestHandleShortPartReturnsUnexpectedEOF(t *testing.T) {
	err := runHandle(t, func(w *multipart.Writer) {
		addPart(t, w, 8, []byte("abcd"))
	})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}
