package api

import (
	"bufio"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// What an iPhone sends to a carried-over accessory whose instance name has a
// space: net/http refuses it as is (the control), and serves it through the
// listener wrapper.
func TestHomeKitHostHeader(t *testing.T) {
	const req = "POST /pair-verify HTTP/1.1\r\nHost: Hallway\\0320CCD._hap._tcp.local\r\n" +
		"Content-Length: 2\r\nContent-Type: application/pairing+tlv8\r\n\r\nok"

	status := func(wrap bool) string {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		if wrap {
			ln = hostEscapeListener{ln}
		}
		srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Hallway-0CCD._hap._tcp.local", r.Host)
		})}
		go func() { _ = srv.Serve(ln) }()
		defer srv.Close()

		conn, err := net.Dial("tcp", ln.Addr().String())
		require.NoError(t, err)
		defer conn.Close()
		_, err = conn.Write([]byte(req))
		require.NoError(t, err)
		line, err := bufio.NewReader(conn).ReadString('\n')
		require.NoError(t, err)
		return strings.TrimSpace(line)
	}

	require.Contains(t, status(false), "400")
	require.Equal(t, "HTTP/1.1 200 OK", status(true))
}

// Bytes that are not a request line pass untouched.
func TestHostEscapesLeaveFramesAlone(t *testing.T) {
	frame := []byte{0x20, 0x00, 'H', 'o', 's', 't', ':', ' ', '\\', '0', '3', '2'}
	p := append([]byte{}, frame...)
	require.Equal(t, len(frame), rewriteHostEscapes(p, len(p)))
	require.Equal(t, frame, p)
}
