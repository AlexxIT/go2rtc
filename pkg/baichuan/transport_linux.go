package baichuan

import (
	"net"
	"sync"
	"syscall"
)

type quickACKConn struct {
	net.Conn
	mu    sync.Mutex
	force func() error
}

func newQuickACKConn(conn net.Conn) net.Conn {
	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		return conn
	}
	raw, err := tcp.SyscallConn()
	if err != nil {
		return conn
	}
	var optionErr error
	control := func(fd uintptr) {
		optionErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_QUICKACK, 1)
	}
	return &quickACKConn{Conn: conn, force: func() error {
		if err := raw.Control(control); err != nil {
			return err
		}
		return optionErr
	}}
}

func (c *quickACKConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		// QUICKACK is temporary; rearm after reading without changing media errors.
		c.mu.Lock()
		if c.force != nil && c.force() != nil {
			c.force = nil
		}
		c.mu.Unlock()
	}
	return n, err
}
