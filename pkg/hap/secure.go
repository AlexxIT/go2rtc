package hap

import (
	"encoding/binary"
	"github.com/brutella/hap/chacha20poly1305"
	"github.com/brutella/hap/hkdf"
	"net"
	"sync"
)

type Secure struct {
	Conn net.Conn

	encryptKey   [32]byte
	decryptKey   [32]byte
	encryptCount uint64
	decryptCount uint64

	mx sync.Mutex
}

func NewSecure(sharedKey [32]byte, isServer bool) (*Secure, error) {
	salt := []byte("Control-Salt")

	key1, err := hkdf.Sha512(
		sharedKey[:], salt, []byte("Control-Read-Encryption-Key"),
	)
	if err != nil {
		return nil, err
	}

	key2, err := hkdf.Sha512(
		sharedKey[:], salt, []byte("Control-Write-Encryption-Key"),
	)
	if err != nil {
		return nil, err
	}

	if isServer {
		return &Secure{encryptKey: key1, decryptKey: key2}, nil
	} else {
		return &Secure{encryptKey: key2, decryptKey: key1}, nil
	}
}

func (s *Secure) Read(b []byte) (n int, err error) {
	for {
		var length uint16
		if err = binary.Read(s.Conn, binary.LittleEndian, &length); err != nil {
			return
		}

		var enc = make([]byte, length)
		if err = binary.Read(s.Conn, binary.LittleEndian, &enc); err != nil {
			return
		}

		var mac [16]byte
		if err = binary.Read(s.Conn, binary.LittleEndian, &mac); err != nil {
			return
		}

		var nonce [8]byte
		binary.LittleEndian.PutUint64(nonce[:], s.decryptCount)
		s.decryptCount++

		bLength := make([]byte, 2)
		binary.LittleEndian.PutUint16(bLength, length)

		var msg []byte
		if msg, err = chacha20poly1305.DecryptAndVerify(
			s.decryptKey[:], nonce[:], enc, mac, bLength,
		); err != nil {
			return
		}

		n += copy(b[n:], msg)

		// Finish when all bytes fit in b
		if length < packetLengthMax {
			//fmt.Printf(">>>%s>>>\n", b[:n])
			return
		}
	}
}

func (s *Secure) Write(b []byte) (n int, err error) {
	s.mx.Lock()
	defer s.mx.Unlock()

	var packetLen = len(b)
	for {
		if packetLen > packetLengthMax {
			packetLen = packetLengthMax
		}

		//fmt.Printf("<<<%s<<<\n", b[:packetLen])

		var nonce [8]byte
		binary.LittleEndian.PutUint64(nonce[:], s.encryptCount)
		s.encryptCount++

		bLength := make([]byte, 2)
		binary.LittleEndian.PutUint16(bLength, uint16(packetLen))

		var enc []byte
		var mac [16]byte
		enc, mac, err = chacha20poly1305.EncryptAndSeal(
			s.encryptKey[:], nonce[:], b[:packetLen], bLength[:],
		)
		if err != nil {
			return
		}

		enc = append(bLength, enc...)
		enc = append(enc, mac[:]...)
		if _, err = s.Conn.Write(enc); err != nil {
			return
		}

		n += packetLen

		if packetLen == packetLengthMax {
			b = b[packetLengthMax:]
			packetLen = len(b)
		} else {
			break
		}
	}

	return
}

const (
	// packetLengthMax is the max length of encrypted packets
	packetLengthMax = 0x400
)
