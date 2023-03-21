package mqtt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"
)

const Timeout = time.Second * 5

type Client struct {
	conn net.Conn
	mid  uint16
}

func NewClient(conn net.Conn) *Client {
	return &Client{conn: conn, mid: 2}
}

func (c *Client) Connect(clientID, username, password string) (err error) {
	if err = c.conn.SetDeadline(time.Now().Add(Timeout)); err != nil {
		return
	}

	msg := NewConnect(clientID, username, password)
	if _, err = c.conn.Write(msg.b); err != nil {
		return
	}

	b := make([]byte, 4)
	if _, err = io.ReadFull(c.conn, b); err != nil {
		return
	}

	if !bytes.Equal(b, []byte{CONNACK, 2, 0, 0}) {
		return errors.New("wrong login")
	}

	return
}

func (c *Client) Subscribe(topic string) (err error) {
	if err = c.conn.SetDeadline(time.Now().Add(Timeout)); err != nil {
		return
	}

	c.mid++
	msg := NewSubscribe(c.mid, topic, 1)
	_, err = c.conn.Write(msg.b)
	return
}

func (c *Client) Publish(topic string, payload []byte) (err error) {
	if err = c.conn.SetDeadline(time.Now().Add(Timeout)); err != nil {
		return
	}

	c.mid++
	msg := NewPublishQOS1(c.mid, topic, payload)
	_, err = c.conn.Write(msg.b)
	return
}

func (c *Client) Read() (string, []byte, error) {
	if err := c.conn.SetDeadline(time.Now().Add(Timeout)); err != nil {
		return "", nil, err
	}

	b := make([]byte, 1)
	if _, err := io.ReadFull(c.conn, b); err != nil {
		return "", nil, err
	}

	size, err := ReadLen(c.conn)
	if err != nil {
		return "", nil, err
	}

	b0 := b[0]
	b = make([]byte, size)
	if _, err = io.ReadFull(c.conn, b); err != nil {
		return "", nil, err
	}

	if b0&0xF0 != PUBLISH {
		return "", nil, nil
	}

	i := binary.BigEndian.Uint16(b)
	if uint32(i) > size {
		return "", nil, errors.New("wrong topic size")
	}

	b = b[2:]

	if qos := (b0 >> 1) & 0b11; qos == 0 {
		return string(b[:i]), b[i:], nil
	}

	// response with packet ID
	_, _ = c.conn.Write([]byte{PUBACK, 2, b[i], b[i+1]})

	return string(b[2:i]), b[i+2:], nil
}

func (c *Client) Close() error {
	// TODO: Teardown
	return c.conn.Close()
}
