package unifiprotect

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	protectmedia "github.com/AlexxIT/go2rtc/pkg/unifiprotect"
	"github.com/stretchr/testify/require"
)

func TestSharedListenerRoutesControl(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	shared := newSharedListener(listener)
	defer shared.Close()
	go shared.serve(newManager("", listener.Addr().(*net.TCPAddr).Port, "test", "controller-id"))

	type acceptResult struct {
		conn net.Conn
		err  error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		conn, err := shared.Accept()
		accepted <- acceptResult{conn: conn, err: err}
	}()

	client, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer client.Close()
	payload := []byte{tlsHandshakeRecord, 3, 3, 0, 1, 0}
	_, err = client.Write(payload)
	require.NoError(t, err)

	select {
	case result := <-accepted:
		require.NoError(t, result.err)
		require.NotNil(t, result.conn)
		defer result.conn.Close()
		got := make([]byte, len(payload))
		_, err = io.ReadFull(result.conn, got)
		require.NoError(t, err)
		require.Equal(t, payload, got)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for control connection")
	}
}

func TestSharedListenerRoutesMedia(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	shared := newSharedListener(listener)
	defer shared.Close()

	m := newManager("", listener.Addr().(*net.TCPAddr).Port, "test", "controller-id")
	const token = "shared-listener-test"
	req := &streamRequest{
		token:          token,
		cameraIP:       "127.0.0.1",
		candidates:     make(chan candidate, 1),
		candidatesDone: make(chan struct{}),
	}
	m.pending[token] = req
	go shared.serve(m)

	client, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer client.Close()
	media := syntheticMedia(token)
	_, err = io.Copy(client, bytes.NewReader(media))
	require.NoError(t, err)

	select {
	case selected := <-req.candidates:
		defer selected.rd.Close()
		tag, err := protectmedia.NewReader(selected.rd).ReadTag()
		require.NoError(t, err)
		metadata, ok, err := protectmedia.ParseMetadata(tag)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, token, metadata.StreamName)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for media connection")
	}
}

func TestSharedListenerCloseUnblocksAccept(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	shared := newSharedListener(listener)

	accepted := make(chan error, 1)
	go func() {
		_, err := shared.Accept()
		accepted <- err
	}()
	require.NoError(t, shared.Close())

	select {
	case err := <-accepted:
		require.ErrorIs(t, err, net.ErrClosed)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for listener close")
	}
}
