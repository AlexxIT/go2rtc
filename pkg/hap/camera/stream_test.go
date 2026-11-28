package camera

import (
	"io"
	"net"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
	"github.com/stretchr/testify/require"
)

func TestExchangeEndpointsAddressFamily(t *testing.T) {
	for _, test := range []struct {
		name     string
		addr     string
		wantAddr string
		version  byte
	}{
		{"IPv4", "192.0.2.1", "192.0.2.1", 0},
		{"IPv6", "2001:db8::1", "2001:db8::1", 1},
		{"mapped IPv4", "::ffff:192.0.2.1", "192.0.2.1", 0},
		{"link local", "fe80::1", "fe80::1", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn, peer := net.Pipe()
			defer conn.Close()
			require.NoError(t, peer.Close())
			char := &hap.Character{IID: 1, Type: TypeSetupEndpoints, Format: hap.FormatTLV8}
			stream := &Stream{
				id:      "test-session",
				client:  &hap.Client{DeviceAddress: "camera.test:80", Conn: conn},
				service: &hap.Service{Characters: []*hap.Character{char}},
			}
			video := &srtp.Session{Local: &srtp.Endpoint{Addr: test.addr, Port: 5000}}
			audio := &srtp.Session{Local: &srtp.Endpoint{Addr: test.addr, Port: 5002}}
			require.ErrorIs(t, stream.ExchangeEndpoints(video, audio), io.ErrClosedPipe)
			var request SetupEndpointsRequest
			require.NoError(t, char.ReadTLV8(&request))
			require.Equal(t, test.version, request.Address.IPVersion)
			require.Equal(t, test.wantAddr, request.Address.IPAddr)
			require.Equal(t, uint16(5000), request.Address.VideoRTPPort)
			require.Equal(t, uint16(5002), request.Address.AudioRTPPort)
		})
	}
}

func TestExchangeEndpointsInvalidAddress(t *testing.T) {
	for _, addr := range []string{"", "camera.test", "fe80::1%eth0"} {
		t.Run(addr, func(t *testing.T) {
			stream := &Stream{}
			video := &srtp.Session{Local: &srtp.Endpoint{Addr: addr}}
			require.ErrorContains(t, stream.ExchangeEndpoints(video, nil), "invalid local IP address")
		})
	}
}
