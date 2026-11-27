package tapo

import (
	"io"
	"net"
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
