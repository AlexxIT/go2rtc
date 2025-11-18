// Package hds - HomeKit Data Stream
package hds

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/chacha20poly1305"
	"github.com/AlexxIT/go2rtc/pkg/hap/hkdf"
)

func NewConn(conn net.Conn, key []byte, salt string, controller bool) (*Conn, error) {
	writeKey, err := hkdf.Sha512(key, salt, "HDS-Write-Encryption-Key")
	if err != nil {
		return nil, err
	}

	readKey, err := hkdf.Sha512(key, salt, "HDS-Read-Encryption-Key")
	if err != nil {
		return nil, err
	}

	c := &Conn{
		conn: conn,
		rd:   bufio.NewReaderSize(conn, 32*1024),
		wr:   bufio.NewWriterSize(conn, 32*1024),
	}

	if controller {
		c.decryptKey, c.encryptKey = readKey, writeKey
	} else {
		c.decryptKey, c.encryptKey = writeKey, readKey
	}

	return c, nil
}

type Conn struct {
	conn net.Conn

	rd *bufio.Reader
	wr *bufio.Writer

	decryptKey []byte
	encryptKey []byte
	decryptCnt uint64
	encryptCnt uint64

	recv int
	send int
}

func (c *Conn) MarshalJSON() ([]byte, error) {
	conn := core.Connection{
		ID:         core.ID(c),
		FormatName: "homekit",
		Protocol:   "hds",
		RemoteAddr: c.conn.RemoteAddr().String(),
		Recv:       c.recv,
		Send:       c.send,
	}
	return json.Marshal(conn)
}

func (c *Conn) read() (b []byte, err error) {
	verify := make([]byte, 4)
	if _, err = io.ReadFull(c.rd, verify); err != nil {
		return
	}

	n := int(binary.BigEndian.Uint32(verify) & 0xFFFFFF)

	ciphertext := make([]byte, n+hap.Overhead)
	if _, err = io.ReadFull(c.rd, ciphertext); err != nil {
		return
	}

	nonce := make([]byte, hap.NonceSize)
	binary.LittleEndian.PutUint64(nonce, c.decryptCnt)
	c.decryptCnt++

	c.recv += n

	return chacha20poly1305.DecryptAndVerify(c.decryptKey, ciphertext[:0], nonce, ciphertext, verify)
}

func (c *Conn) Read(p []byte) (n int, err error) {
	b, err := c.read()
	if err != nil {
		return 0, err
	}
	n = copy(p, b)
	if len(b) > n {
		err = errors.New("hds: read buffer too small")
	}
	return
}

func (c *Conn) WriteTo(w io.Writer) (int64, error) {
	var total int64
	for {
		b, err := c.read()
		if err != nil {
			return total, err
		}

		n, err := w.Write(b)
		total += int64(n)
		if err != nil {
			return total, err
		}
	}
}

func (c *Conn) Write(b []byte) (n int, err error) {
	n = len(b)

	if n > 0xFFFFFF {
		return 0, errors.New("hds: write buffer too big")
	}

	verify := make([]byte, 4)
	binary.BigEndian.PutUint32(verify, 0x01000000|uint32(n))
	if _, err = c.wr.Write(verify); err != nil {
		return
	}

	nonce := make([]byte, hap.NonceSize)
	binary.LittleEndian.PutUint64(nonce, c.encryptCnt)
	c.encryptCnt++

	buf := make([]byte, n+hap.Overhead)
	if _, err = chacha20poly1305.EncryptAndSeal(c.encryptKey, buf[:0], nonce, b, verify); err != nil {
		return
	}

	if _, err = c.wr.Write(buf); err != nil {
		return
	}

	err = c.wr.Flush()

	c.send += n
	return
}

func (c *Conn) Close() error {
	return c.conn.Close()
}

func (c *Conn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *Conn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}
