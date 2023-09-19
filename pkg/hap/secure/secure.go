package secure

import (
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

	if isClient {
		return &Conn{conn: conn, encryptKey: key2, decryptKey: key1}, nil
	} else {
		return &Conn{conn: conn, encryptKey: key1, decryptKey: key2}, nil
	}
}

const (
	// PacketSizeMax is the max length of encrypted packets
	PacketSizeMax = 0x400

	VerifySize = 2
	NonceSize  = 8
	Overhead   = 16 // chacha20poly1305.Overhead
)

func (c *Conn) Read(b []byte) (n int, err error) {
	verify := make([]byte, VerifySize) // = packet length
	buf := make([]byte, PacketSizeMax+Overhead)
	nonce := make([]byte, NonceSize)

	for {
		if len(b) < PacketSizeMax {
			return
		}

		if _, err = io.ReadFull(c.conn, verify); err != nil {
			return
		}

		size := binary.LittleEndian.Uint16(verify)
		ciphertext := buf[:size+Overhead]

		if _, err = io.ReadFull(c.conn, ciphertext); err != nil {
			return
		}

		binary.LittleEndian.PutUint64(nonce, c.decryptCnt)
		c.decryptCnt++

		// put decrypted text to b's end
		_, err = chacha20poly1305.DecryptAndVerify(c.decryptKey, b[:0], nonce, ciphertext, verify)
		if err != nil {
			return
		}

		n += int(size) // plaintext size

		// Finish when all bytes fit in b
		if size < PacketSizeMax {
			return
		}

		b = b[size:]
	}
}

func (c *Conn) Write(b []byte) (n int, err error) {
	c.mx.Lock()
	defer c.mx.Unlock()

	nonce := make([]byte, NonceSize)
	buf := make([]byte, NonceSize+PacketSizeMax+Overhead)
	verify := buf[:VerifySize] // part of write buffer

	for {
		size := len(b)
		if size > PacketSizeMax {
			size = PacketSizeMax
		}

		binary.LittleEndian.PutUint16(verify, uint16(size))

		binary.LittleEndian.PutUint64(nonce, c.encryptCnt)
		c.encryptCnt++

		// put encrypted text to writing buffer just after size (2 bytes)
		_, err = chacha20poly1305.EncryptAndSeal(c.encryptKey, buf[2:2], nonce, b[:size], verify)
		if err != nil {
			return
		}

		if _, err = c.conn.Write(buf[:VerifySize+size+Overhead]); err != nil {
			return
		}

		n += size // plaintext size

		if size < PacketSizeMax {
			break
		}

		b = b[PacketSizeMax:]
	}

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
