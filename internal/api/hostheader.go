package api

import (
	"bytes"
	"net"
	"regexp"
)

// A HomeKit controller puts the accessory's DNS-SD instance name in the Host
// header with DNS escapes ("Hallway\0320CCD._hap._tcp.local"), and net/http
// answers a backslash in Host with 400 before any handler runs. So the Host
// line of each plaintext request on the API listener has its escapes rewritten
// to '-'. A HAP frame after pair-verify starts with its little-endian length
// (at most 1024), so it is never mistaken for a request line.

var hostEscape = regexp.MustCompile(`\\[0-9]{3}`)

type hostEscapeListener struct{ net.Listener }

func (l hostEscapeListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &hostEscapeConn{Conn: conn}, nil
}

type hostEscapeConn struct{ net.Conn }

func (c *hostEscapeConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		n = rewriteHostEscapes(p, n)
	}
	return n, err
}

// rewriteHostEscapes rewrites p[:n] in place and returns its new length.
func rewriteHostEscapes(p []byte, n int) int {
	b := p[:n]
	if !bytes.HasPrefix(b, []byte("POST ")) && !bytes.HasPrefix(b, []byte("GET ")) && !bytes.HasPrefix(b, []byte("PUT ")) {
		return n
	}
	i := bytes.Index(b, []byte("\r\nHost: "))
	if i < 0 {
		return n
	}
	start := i + len("\r\nHost: ")
	end := bytes.Index(b[start:], []byte("\r\n"))
	if end < 0 {
		return n
	}
	end += start
	host := hostEscape.ReplaceAll(b[start:end], []byte("-"))
	if len(host) == end-start {
		return n
	}
	out := make([]byte, 0, n)
	out = append(out, b[:start]...)
	out = append(out, host...)
	out = append(out, b[end:]...)
	return copy(p, out)
}
