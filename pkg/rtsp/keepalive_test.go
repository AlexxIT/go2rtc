package rtsp

import (
	"bufio"
	"context"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKeepaliveMethodString(t *testing.T) {
	require.Equal(t, MethodOptions, KeepaliveOptions.String())
	require.Equal(t, MethodGetParameter, KeepaliveGetParameter.String())
	// zero value must keep the historic behaviour
	var zero KeepaliveMethod
	require.Equal(t, MethodOptions, zero.String())
}

// TestKeepaliveMethod checks the request the keepalive loop puts on the wire:
// OPTIONS by default, GET_PARAMETER when the source asks for it. The ticker is
// driven directly so the test doesn't wait on a real session timeout.
func TestKeepaliveMethod(t *testing.T) {
	tests := []struct {
		name   string
		method KeepaliveMethod
		expect string
	}{
		{"default", KeepaliveOptions, MethodOptions},
		{"get_parameter", KeepaliveGetParameter, MethodGetParameter},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()

			rawURL := "rtsp://localhost/stream"
			u, err := url.Parse(rawURL)
			require.Nil(t, err)

			conn := NewClient(rawURL)
			conn.URL = u
			conn.conn = client
			conn.KeepaliveMethod = test.method

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go conn.handleKeepalive(ctx, time.Millisecond)

			require.Nil(t, server.SetReadDeadline(time.Now().Add(time.Second)))

			line, err := bufio.NewReader(server).ReadString('\n')
			require.Nil(t, err)
			require.Equal(t, test.expect+" "+rawURL+" "+ProtoRTSP+"\r\n", line)
		})
	}
}
