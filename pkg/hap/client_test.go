package hap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeviceHost(t *testing.T) {
	tests := []struct{ address, host string }{
		{"192.168.1.50:51826", "192.168.1.50"},
		{"[fd00::60]:51827", "fd00::60"},
		{"[fe80::1%eth0]:51826", "fe80::1%eth0"},
		{"192.168.1.50", "192.168.1.50"}, // no port
	}
	for _, test := range tests {
		c := &Client{DeviceAddress: test.address}
		require.Equal(t, test.host, c.DeviceHost(), test.address)
	}
}
