package rtmp

import (
	"io"
	"net"
	"net/url"
	"testing"
)

func TestNewClientURLComponents(t *testing.T) {
	for _, ca := range []struct {
		name   string
		rawURL string
		app    string
		stream string
		tcURL  string
	}{
		{
			name:   "without stream",
			rawURL: "rtmp://localhost:1935/siteA?user",
			app:    "siteA?user",
			stream: "",
			tcURL:  "rtmp://localhost:1935/siteA?user",
		},
		{
			name:   "with stream",
			rawURL: "rtmp://localhost:1935/siteA/cam1?user",
			app:    "siteA",
			stream: "cam1?user",
			tcURL:  "rtmp://localhost:1935/siteA",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			client := newClientWithHandshake(t, ca.rawURL)

			if got := client.App; got != ca.app {
				t.Errorf("App = %q, want %q", got, ca.app)
			}
			if got := client.Stream; got != ca.stream {
				t.Errorf("Stream = %q, want %q", got, ca.stream)
			}
			if got := client.url; got != ca.tcURL {
				t.Errorf("tcUrl = %q, want %q", got, ca.tcURL)
			}
		})
	}
}

func newClientWithHandshake(t *testing.T, rawURL string) *Conn {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close() })
	t.Cleanup(func() { _ = serverConn.Close() })

	serverErr := make(chan error, 1)
	go func() {
		defer serverConn.Close()

		// Complete the RTMP simple handshake.
		b := make([]byte, 1+1536)
		if _, err := io.ReadFull(serverConn, b); err != nil {
			serverErr <- err
			return
		}
		if _, err := serverConn.Write(b); err != nil {
			serverErr <- err
			return
		}
		if _, err := io.ReadFull(serverConn, b[1:]); err != nil {
			serverErr <- err
			return
		}
		if _, err := serverConn.Write(b[1:]); err != nil {
			serverErr <- err
			return
		}

		// NewClient sends the initial chunk size immediately after handshake.
		packet := make([]byte, 16)
		_, err := io.ReadFull(serverConn, packet)
		serverErr <- err
	}()

	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}

	client, err := NewClient(clientConn, u)
	if err != nil {
		t.Fatal(err)
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("RTMP test server: %v", err)
	}
	return client
}
