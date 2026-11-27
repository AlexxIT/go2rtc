package hap

import (
	"bufio"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// packetConn returns each packet with a separate Read call
type packetConn struct {
	net.Conn
	packets []string
}

func (c *packetConn) Read(b []byte) (int, error) {
	if len(c.packets) == 0 {
		return 0, io.EOF
	}
	n := copy(b, c.packets[0])
	c.packets = c.packets[1:]
	return n, nil
}

func (c *packetConn) Write(b []byte) (int, error) {
	return len(b), nil
}

func TestGetAccessoriesChunked(t *testing.T) {
	conn := &packetConn{packets: []string{
		"HTTP/1.1 200 OK\r\nContent-Type: application/hap+json\r\nTransfer-Encoding: chunked\r\n\r\n" +
			"12\r\n{\"accessories\":[]}\r\n",
		// last chunk in a separate packet
		"0\r\n\r\n",
		"HTTP/1.1 200 OK\r\nContent-Type: application/hap+json\r\nContent-Length: 18\r\n\r\n" +
			"{\"accessories\":[]}",
	}}

	c := &Client{Conn: conn, reader: bufio.NewReader(conn)}

	_, err := c.GetAccessories()
	require.Nil(t, err)

	// next request should not get the last chunk of the previous response
	_, err = c.GetAccessories()
	require.Nil(t, err)
}
