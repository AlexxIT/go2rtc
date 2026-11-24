package mdns

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscovery(t *testing.T) {
	onentry := func(entry *ServiceEntry) bool {
		return true
	}
	err := Discovery(ServiceHAP, onentry)
	//err := Discovery("_ewelink._tcp.local.", time.Second, onentry)
	// err := Discovery("_googlecast._tcp.local.", time.Second, onentry)
	require.Nil(t, err)
}

func TestServiceEntryAddr(t *testing.T) {
	entry := &ServiceEntry{IP: net.ParseIP("192.168.1.50"), Port: 51826}
	require.Equal(t, "192.168.1.50:51826", entry.Addr())

	// IPv6 addresses need brackets to be dialable
	entry = &ServiceEntry{IP: net.ParseIP("fd00::60"), Port: 51827}
	require.Equal(t, "[fd00::60]:51827", entry.Addr())
}
