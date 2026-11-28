package streams

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendDOTQuotesHost(t *testing.T) {
	tests := []struct {
		name string
		conn conn
		host string
	}{
		{"ipv4", conn{ID: 1, Protocol: "rtsp", RemoteAddr: "192.168.1.50:554"}, `"192.168.1.50"`},
		{"ipv6", conn{ID: 1, Protocol: "rtsp", RemoteAddr: "[fe80::1]:554"}, `"fe80::1"`},
		{"hostname", conn{ID: 1, Protocol: "rtsp", RemoteAddr: "cam.example.com:554"}, `"cam.example.com"`},
		{"pipe", conn{ID: 1, Protocol: "pipe"}, `"127.0.0.1"`},
		{"unknown", conn{ID: 1, Protocol: "rtsp"}, `"unknown"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.conn

			dot := string(c.appendDOT(nil, "producer"))
			require.Contains(t, dot, tt.host+" [group=host];\n")
			require.Contains(t, dot, tt.host+" -> 1 [label=")

			dot = string(c.appendDOT(nil, "consumer"))
			require.Contains(t, dot, "1 -> "+tt.host+" [label=")
		})
	}
}
