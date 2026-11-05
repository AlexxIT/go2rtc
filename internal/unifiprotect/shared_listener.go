package unifiprotect

import (
	"bytes"
	"io"
	"net"
	"sync"
	"time"
)

const tlsHandshakeRecord byte = 22

// sharedListener accepts control connections through net.Listener while
// sending non-TLS connections directly to the media handler. TLS handshake
// records and FLV tags have distinct first bytes, so no protocol bytes need to
// be consumed while selecting the destination.
type sharedListener struct {
	net.Listener

	control   chan net.Conn
	done      chan struct{}
	closeOnce sync.Once
}

func newSharedListener(ln net.Listener) *sharedListener {
	return &sharedListener{
		Listener: ln,
		control:  make(chan net.Conn),
		done:     make(chan struct{}),
	}
}

func (l *sharedListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.control:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *sharedListener) Close() (err error) {
	l.closeOnce.Do(func() {
		close(l.done)
		err = l.Listener.Close()
	})
	return
}

func (l *sharedListener) serve(m *Manager) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			_ = l.Close()
			return
		}
		go l.route(conn, m)
	}
}

func (l *sharedListener) route(conn net.Conn, m *Manager) {
	owned := true
	defer func() {
		if owned {
			_ = conn.Close()
		}
	}()

	if err := conn.SetReadDeadline(time.Now().Add(probeTimeout)); err != nil {
		return
	}
	var first [1]byte
	if _, err := io.ReadFull(conn, first[:]); err != nil {
		log.Debug().Err(err).Str("remote", conn.RemoteAddr().String()).Msg("[unifi-protect] shared connection")
		return
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return
	}

	conn = &replayConn{
		Conn:   conn,
		Reader: io.MultiReader(bytes.NewReader(first[:]), conn),
	}
	if first[0] != tlsHandshakeRecord {
		owned = false
		m.handleMedia(conn)
		return
	}

	select {
	case l.control <- conn:
		owned = false
	case <-l.done:
	}
}

type replayConn struct {
	net.Conn
	io.Reader
}

func (c *replayConn) Read(p []byte) (int, error) {
	return c.Reader.Read(p)
}
