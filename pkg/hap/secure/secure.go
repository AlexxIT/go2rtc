package secure

import (
	"bufio"
	"encoding/binary"
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
	rb []byte // temporary reading buffer

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
		rd:   bufio.NewReaderSize(conn, BufferSize),
		wr:   bufio.NewWriterSize(conn, BufferSize),
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

	BufferSize = 2 + 0xFFFF + Overhead
)

func (c *Conn) Read(b []byte) (n int, err error) {
	// something in reading buffer
	if len(c.rb) > 0 {
		n = copy(b, c.rb)
		c.rb = c.rb[n:]
		return
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

	// buffer size enought for direct reading
	if n <= cap(b) {
		_, err = chacha20poly1305.DecryptAndVerify(c.decryptKey, b[:0], nonce, ciphertext, verify)
		return
	}

	// reading to temporary buffer
	c.rb = make([]byte, 0, n)
	if _, err = chacha20poly1305.DecryptAndVerify(c.decryptKey, c.rb, nonce, ciphertext, verify); err != nil {
		return
	}
	return c.Read(b)
}

func (c *Conn) Write(b []byte) (n int, err error) {
	var ciphertext []byte
	var nonce = make([]byte, NonceSize)
	var verify = make([]byte, VerifySize)

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

		if cap(ciphertext) < size+Overhead {
			ciphertext = make([]byte, size+Overhead)
		}

		_, err = chacha20poly1305.EncryptAndSeal(c.encryptKey, ciphertext[:0], nonce, b[:size], verify)
		if err != nil {
			return
		}

		if _, err = c.wr.Write(ciphertext); err != nil {
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
