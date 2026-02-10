package debug

import (
	"bytes"
	"math/rand"
	"net"
)

type badConn struct {
	net.Conn
	delay int
	buf   []byte
}

func NewBadConn(conn net.Conn) net.Conn {
	return &badConn{Conn: conn}
}

const (
	missChance  = 0.05
	delayChance = 0.1
)

func (c *badConn) Read(b []byte) (n int, err error) {
	if rand.Float32() < missChance {
		if _, err = c.Conn.Read(b); err != nil {
			return
		}
		//log.Printf("bad conn: miss")
	}

	if c.delay > 0 {
		if c.delay--; c.delay == 0 {
			n = copy(b, c.buf)
			return
		}
	} else if rand.Float32() < delayChance {
		if n, err = c.Conn.Read(b); err != nil {
			return
		}
		c.delay = 1 + rand.Intn(5)
		c.buf = bytes.Clone(b[:n])
		//log.Printf("bad conn: delay %d", c.delay)
	}

	return c.Conn.Read(b)
}
