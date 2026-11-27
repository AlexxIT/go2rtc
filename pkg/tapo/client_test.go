package tapo

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// runHandle starts Client.Handle on one end of a net.Pipe, lets write feed the
// other end, and returns the error from Handle (or fails on timeout).
func runHandle(t *testing.T, write func(w *multipart.Writer)) error {
	t.Helper()

	c1, c2 := net.Pipe()
	defer c2.Close()

	c := &Client{
		conn1: c1,
		decrypt: func(b []byte) ([]byte, error) {
			return b, nil
		},
	}
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
		addPart(t, w, 2, []byte("abcd"))
	})
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestHandleNegativeContentLength(t *testing.T) {
	err := runHandle(t, func(w *multipart.Writer) {
		addPart(t, w, -1, nil)
	})
	if err == nil || err.Error() != "negative Content-Length" {
		t.Fatalf("expected negative Content-Length error, got %v", err)
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

// https://github.com/AlexxIT/go2rtc/issues/2690
func TestGetTrackAfterStart(t *testing.T) {
	conn, camera := net.Pipe()
	t.Cleanup(func() { _ = camera.Close() })
	go func() { _, _ = io.Copy(io.Discard, camera) }() // reads requests, never answers

	// producer started for the backchannel only: Handle owns conn1, session1 is empty
	client := &Client{conn1: conn}
	go func() { _ = client.Start() }()
	time.Sleep(100 * time.Millisecond)

	media := client.GetMedias()[0]
	done := make(chan error, 1)
	go func() {
		_, err := client.GetTrack(media, media.Codecs[0])
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("GetTrack blocked on conn1 while Handle is reading it")
	}
}

func TestDecryptRejectsInvalidBlockSize(t *testing.T) {
	client := newDecryptTestClient(t)

	for _, chunk := range [][]byte{nil, make([]byte, aes.BlockSize-1)} {
		_, err := client.decrypt(chunk)
		require.EqualError(t, err, "tapo: invalid encryption")
	}
}

func TestDecryptRejectsInvalidPaddingSize(t *testing.T) {
	for _, test := range []struct {
		name    string
		padSize byte
	}{
		{name: "zero", padSize: 0},
		{name: "larger than block", padSize: aes.BlockSize + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newDecryptTestClient(t)
			key := md5.Sum([]byte("test-nonce:test-password"))
			iv := md5.Sum([]byte("test-user:test-nonce"))
			block, err := aes.NewCipher(key[:])
			require.NoError(t, err)

			plain := make([]byte, aes.BlockSize)
			plain[len(plain)-1] = test.padSize
			chunk := make([]byte, len(plain))
			cipher.NewCBCEncrypter(block, iv[:]).CryptBlocks(chunk, plain)

			_, err = client.decrypt(chunk)
			require.EqualError(t, err, "tapo: invalid encryption")
		})
	}
}

func newDecryptTestClient(t *testing.T) *Client {
	t.Helper()

	res := &http.Response{Header: make(http.Header)}
	res.Header.Set("Key-Exchange", `nonce="test-nonce"`)
	client := &Client{}
	client.newDectypter(res, "vigi", "test-user", "test-password")
	return client
}