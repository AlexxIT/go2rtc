package baichuan

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
)

type encryption uint8

const (
	encryptionNone encryption = iota
	encryptionBC
	encryptionAES
)

var (
	bcKey = [...]byte{0x1f, 0x2d, 0x3c, 0x4b, 0x5a, 0x69, 0x78, 0xff}
	aesIV = []byte("0123456789abcdef")
)

type cipherState struct {
	mode   encryption
	aesKey [aes.BlockSize]byte
	block  cipher.Block
	hasKey bool
}

func (s *cipherState) setAESKey(key [aes.BlockSize]byte) {
	s.aesKey = key
	s.block, _ = aes.NewCipher(key[:])
	s.hasKey = true
}

func modernMD5(value string) string {
	sum := md5.Sum([]byte(value)) // Baichuan uses the first 31 uppercase hex digits.
	const digits = "0123456789ABCDEF"

	out := make([]byte, 31)
	for i := range out {
		b := sum[i/2]
		if i&1 == 0 {
			out[i] = digits[b>>4]
		} else {
			out[i] = digits[b&0x0f]
		}
	}
	return string(out)
}

func deriveAESKey(nonce, password string) [aes.BlockSize]byte {
	value := modernMD5(nonce + "-" + password)
	var key [aes.BlockSize]byte
	copy(key[:], value)
	return key
}

func (s cipherState) decryptInPlace(channel uint8, b []byte) {
	s.cryptInPlace(channel, b, false)
}

func (s cipherState) cryptInPlace(channel uint8, b []byte, encrypt bool) {
	switch s.mode {
	case encryptionBC:
		for i := range b {
			b[i] ^= bcKey[(int(channel)+i)%len(bcKey)] ^ channel
		}
	case encryptionAES:
		if !s.hasKey {
			for i := range b {
				b[i] ^= bcKey[(int(channel)+i)%len(bcKey)] ^ channel
			}
			return
		}
		block := s.block
		if block == nil {
			block, _ = aes.NewCipher(s.aesKey[:])
		}
		var stream cipher.Stream
		if encrypt {
			stream = cipher.NewCFBEncrypter(block, aesIV)
		} else {
			stream = cipher.NewCFBDecrypter(block, aesIV)
		}
		stream.XORKeyStream(b, b)
	}
}

func negotiatedEncryption(code uint16) (encryption, bool) {
	if code>>8 != 0xdd {
		return 0, false
	}
	switch byte(code) {
	case 0:
		return encryptionNone, true
	case 1, 0x12:
		return encryptionBC, true
	case 2, 3:
		return encryptionAES, true
	default:
		return 0, false
	}
}
