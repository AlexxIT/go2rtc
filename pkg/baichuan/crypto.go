package baichuan

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //#nosec G501
)

var (
	bcXMLKey  = [...]byte{0x1F, 0x2D, 0x3C, 0x4B, 0x5A, 0x69, 0x78, 0xFF}
	udpXMLKey = [...]uint32{
		0x1f2d3c4b,
		0x5a6c7f8d,
		0x38172e4b,
		0x8271635a,
		0x863f1a2b,
		0xa5c6f7d8,
		0x8371e1b4,
		0x17f2d3a5,
	}
	aesIV = []byte("0123456789abcdef")
)

// MD5Modern reproduces Reolink's modern MD5 truncation behavior.
func MD5Modern(input string) string {
	sum := md5.Sum([]byte(input)) //#nosec G501
	return stringifyMD5(sum[:])
}

func stringifyMD5(sum []byte) string {
	const hex = "0123456789ABCDEF"

	out := make([]byte, len(sum)*2)
	for i, b := range sum {
		out[i*2] = hex[b>>4]
		out[i*2+1] = hex[b&0x0F]
	}

	if len(out) > 31 {
		out = out[:31]
	}
	return string(out)
}

// DeriveAESKey builds the AES key used after nonce negotiation.
func DeriveAESKey(nonce string, password string) [16]byte {
	keyPhrase := MD5Modern(nonce + "-" + password)
	var out [16]byte
	copy(out[:], []byte(keyPhrase[:16]))
	return out
}

// BCXOR applies the classic Baichuan XML XOR cipher.
func BCXOR(offset uint8, buf []byte) []byte {
	out := make([]byte, len(buf))
	for i, b := range buf {
		key := bcXMLKey[(int(offset)+i)%len(bcXMLKey)]
		out[i] = b ^ key ^ offset
	}
	return out
}

// UDPXOR applies the UID discovery XOR stream.
func UDPXOR(tid uint32, buf []byte) []byte {
	stream := make([]byte, 0, len(udpXMLKey)*4)
	for _, key := range udpXMLKey {
		value := key + tid
		stream = append(stream, byte(value), byte(value>>8), byte(value>>16), byte(value>>24)) //#nosec G115
	}

	out := make([]byte, len(buf))
	for i, b := range buf {
		out[i] = b ^ stream[i%len(stream)]
	}
	return out
}

func encryptXML(offset uint8, buf []byte, mode EncryptionMode, aesKey [16]byte, hasAESKey bool) []byte {
	switch mode {
	case EncryptionNone:
		return append([]byte(nil), buf...)
	case EncryptionBC:
		return BCXOR(offset, buf)
	case EncryptionAES:
		if !hasAESKey {
			return BCXOR(offset, buf)
		}
		return aesCFB(buf, aesKey, true)
	default:
		return append([]byte(nil), buf...)
	}
}

func decryptXML(offset uint8, buf []byte, mode EncryptionMode, aesKey [16]byte, hasAESKey bool) []byte {
	switch mode {
	case EncryptionNone:
		return append([]byte(nil), buf...)
	case EncryptionBC:
		return BCXOR(offset, buf)
	case EncryptionAES:
		if !hasAESKey {
			return BCXOR(offset, buf)
		}
		return aesCFB(buf, aesKey, false)
	default:
		return append([]byte(nil), buf...)
	}
}

func aesCFB(buf []byte, key [16]byte, encrypt bool) []byte {
	block, _ := aes.NewCipher(key[:])
	out := append([]byte(nil), buf...)

	if encrypt {
		//nolint:staticcheck // CFB is required by the Reolink Baichuan protocol.
		stream := cipher.NewCFBEncrypter(block, aesIV) //#nosec G407
		stream.XORKeyStream(out, out)
		return out
	}

	//nolint:staticcheck // CFB is required by the Reolink Baichuan protocol.
	stream := cipher.NewCFBDecrypter(block, aesIV)
	stream.XORKeyStream(out, out)
	return out
}
