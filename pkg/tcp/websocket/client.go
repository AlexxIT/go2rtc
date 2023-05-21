package websocket

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

const BinaryMessage = 2

type Client struct {
	conn   net.Conn
	remain int
}

func NewClient(conn net.Conn) *Client {
	return &Client{conn: conn}
}

const finalBit = 0x80
const maskBit = 0x80

func (w *Client) Read(b []byte) (n int, err error) {
	if w.remain == 0 {
		b2 := make([]byte, 2)
		if _, err = io.ReadFull(w.conn, b2); err != nil {
			return 0, err
		}

		frameType := b2[0] & 0xF
		w.remain = int(b2[1] & 0x7F)

		switch frameType {
		case BinaryMessage:
		default:
			return 0, fmt.Errorf("unsupported frame type: %d", frameType)
		}

		switch w.remain {
		case 126:
			if _, err = io.ReadFull(w.conn, b2); err != nil {
				return 0, err
			}
			w.remain = int(binary.BigEndian.Uint16(b2))
		case 127:
			b8 := make([]byte, 8)
			if _, err = io.ReadFull(w.conn, b8); err != nil {
				return 0, err
			}
			w.remain = int(binary.BigEndian.Uint64(b8))
		}
	}

	if w.remain > len(b) {
		n, err = io.ReadFull(w.conn, b)
		w.remain -= n
		return
	}

	n, err = io.ReadFull(w.conn, b[:w.remain])
	w.remain = 0

	return
}

func (w *Client) Write(b []byte) (n int, err error) {
	var data []byte
	var start byte

	size := len(b)

	switch {
	case size > 65535:
		start = 10
		data = make([]byte, size+14)
		data[1] = maskBit | 127
		binary.BigEndian.PutUint64(data[2:], uint64(size))
	case size > 125:
		start = 4
		data = make([]byte, size+8)
		data[1] = maskBit | 126
		binary.BigEndian.PutUint16(data[2:], uint16(size))
	default:
		start = 2
		data = make([]byte, size+6)
		data[1] = maskBit | byte(size)
	}

	data[0] = BinaryMessage | finalBit

	mask := data[start : start+4]
	msg := data[start+4:]

	if _, err = cryptorand.Read(mask); err != nil {
		return 0, err
	}

	for i := 0; i < len(b); i++ {
		msg[i] = b[i] ^ mask[i%4]
	}

	return w.conn.Write(data)
}

func (w *Client) Close() error {
	return w.conn.Close()
}

func (w *Client) LocalAddr() net.Addr {
	return w.conn.LocalAddr()
}

func (w *Client) RemoteAddr() net.Addr {
	return w.conn.RemoteAddr()
}

func (w *Client) SetDeadline(t time.Time) error {
	return w.conn.SetDeadline(t)
}

func (w *Client) SetReadDeadline(t time.Time) error {
	return w.conn.SetReadDeadline(t)
}

func (w *Client) SetWriteDeadline(t time.Time) error {
	return w.conn.SetWriteDeadline(t)
}
