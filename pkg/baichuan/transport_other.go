//go:build !linux

package baichuan

import "net"

func newQuickACKConn(conn net.Conn) net.Conn { return conn }
