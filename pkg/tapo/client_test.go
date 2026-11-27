package tapo

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
