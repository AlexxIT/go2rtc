//go:build !linux

package baichuan

import (
	"net"
	"testing"
)

func TestQuickACKUnsupportedPlatform(t *testing.T) {
	conn, peer := net.Pipe()
	defer conn.Close()
	defer peer.Close()
	if newQuickACKConn(conn) != conn {
		t.Fatal("changed unsupported-platform transport")
	}
}
