package secure

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/hap/chacha20poly1305"
	"github.com/AlexxIT/go2rtc/pkg/hap/hkdf"
)

type Conn struct {
	conn net.Conn

	rd *bufio.Reader
	wr *bufio.Writer

	encryptKey []byte
	decryptKey []byte
	encryptCnt uint64
	decryptCnt uint64

	mx sync.Mutex
}

func Client(conn net.Conn, sharedKey []byte, isClient bool) (net.Conn, error) {
	key1, err := hkdf.Sha512(sharedKey, "Control-Salt", "Control-Read-Encryption-Key")
	if err != nil {
		return nil, err
	}

	key2, err := hkdf.Sha512(sharedKey, "Control-Salt", "Control-Write-Encryption-Key")
	if err != nil {
		return nil, err
	}

	c := &Conn{
		conn: conn,
		rd:   bufio.NewReaderSize(conn, 32*1024),
		wr:   bufio.NewWriterSize(conn, 32*1024),
	}

	if isClient {
		c.encryptKey, c.decryptKey = key2, key1
	} else {
		c.encryptKey, c.decryptKey = key1, key2
	}

	return c, nil
}

const (
	// PacketSizeMax is the max length of encrypted packets
	PacketSizeMax = 0x400

	VerifySize = 2
	NonceSize  = 8
	Overhead   = 16 // chacha20poly1305.Overhead
)

func (c *Conn) Read(b []byte) (n int, err error) {
	if cap(b) < PacketSizeMax {
		return 0, errors.New("hap: read buffer is too small")
	}

	verify := make([]byte, 2) // verify = plain message size
	if _, err = io.ReadFull(c.rd, verify); err != nil {
		return
	}

	n = int(binary.LittleEndian.Uint16(verify))
	ciphertext := make([]byte, n+Overhead)

	if _, err = io.ReadFull(c.rd, ciphertext); err != nil {
		return
	}

	nonce := make([]byte, NonceSize)
	binary.LittleEndian.PutUint64(nonce, c.decryptCnt)
	c.decryptCnt++

	_, err = chacha20poly1305.DecryptAndVerify(c.decryptKey, b[:0], nonce, ciphertext, verify)
	return
}

func (c *Conn) Write(b []byte) (n int, err error) {
	buf := make([]byte, 0, PacketSizeMax+Overhead)
	nonce := make([]byte, NonceSize)
	verify := make([]byte, VerifySize)

	for len(b) > 0 {
		size := len(b)
		if size > PacketSizeMax {
			size = PacketSizeMax
		}

		binary.LittleEndian.PutUint16(verify, uint16(size))
		if _, err = c.wr.Write(verify); err != nil {
			return
		}

		binary.LittleEndian.PutUint64(nonce, c.encryptCnt)
		c.encryptCnt++

		_, err = chacha20poly1305.EncryptAndSeal(c.encryptKey, buf, nonce, b[:size], verify)
		if err != nil {
			return
		}

		if _, err = c.wr.Write(buf[:size+Overhead]); err != nil {
			return
		}

		b = b[size:]
		n += size
	}

	err = c.wr.Flush()
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
