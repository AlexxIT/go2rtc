package crypto

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"math/bits"
)

const charlie = "Charlie is the designer of P2P!!"

func TransCodePartial(src []byte) []byte {
	n := len(src)
	tmp := make([]byte, n)
	dst := bytes.Clone(src)
	src16, tmp16, dst16 := src, tmp, dst

	for ; n >= 16; n -= 16 {
		for i := 0; i < 16; i += 4 {
			x := binary.LittleEndian.Uint32(src16[i:])
			binary.LittleEndian.PutUint32(tmp16[i:], bits.RotateLeft32(x, -i-1))
		}
		for i := range 16 {
			dst16[i] = tmp16[i] ^ charlie[i]
		}
		swap(dst16, tmp16, 16)
		for i := 0; i < 16; i += 4 {
			x := binary.LittleEndian.Uint32(tmp16[i:])
			binary.LittleEndian.PutUint32(dst16[i:], bits.RotateLeft32(x, -i-3))
		}
		tmp16, dst16, src16 = tmp16[16:], dst16[16:], src16[16:]
	}

	for i := 0; i < n; i++ {
		tmp16[i] = src16[i] ^ charlie[i]
	}
	swap(tmp16, dst16, n)
	return dst
}

func ReverseTransCodePartial(src []byte) []byte {
	n := len(src)
	tmp := make([]byte, n)
	dst := bytes.Clone(src)
	src16, tmp16, dst16 := src, tmp, dst

	for ; n >= 16; n -= 16 {
		for i := 0; i < 16; i += 4 {
			x := binary.LittleEndian.Uint32(src16[i:])
			binary.LittleEndian.PutUint32(tmp16[i:], bits.RotateLeft32(x, i+3))
		}
		swap(tmp16, dst16, 16)
		for i := range 16 {
			tmp16[i] = dst16[i] ^ charlie[i]
		}
		for i := 0; i < 16; i += 4 {
			x := binary.LittleEndian.Uint32(tmp16[i:])
			binary.LittleEndian.PutUint32(dst16[i:], bits.RotateLeft32(x, i+1))
		}
		tmp16, dst16, src16 = tmp16[16:], dst16[16:], src16[16:]
	}

	swap(src16, tmp16, n)
	for i := 0; i < n; i++ {
		dst16[i] = tmp16[i] ^ charlie[i]
	}
	return dst
}

func TransCodeBlob(src []byte) []byte {
	if len(src) < 16 {
		return TransCodePartial(src)
	}

	dst := make([]byte, len(src))
	header := TransCodePartial(src[:16])
	copy(dst, header)

	if len(src) > 16 {
		if src[3]&1 != 0 { // Partial encryption
			remaining := len(src) - 16
			encryptLen := min(remaining, 48)
			if encryptLen > 0 {
				encrypted := TransCodePartial(src[16 : 16+encryptLen])
				copy(dst[16:], encrypted)
			}
			if remaining > 48 {
				copy(dst[64:], src[64:])
			}
		} else { // Full encryption
			encrypted := TransCodePartial(src[16:])
			copy(dst[16:], encrypted)
		}
	}
	return dst
}

func ReverseTransCodeBlob(src []byte) []byte {
	if len(src) < 16 {
		return ReverseTransCodePartial(src)
	}

	dst := make([]byte, len(src))
	header := ReverseTransCodePartial(src[:16])
	copy(dst, header)

	if len(src) > 16 {
		if dst[3]&1 != 0 { // Partial encryption (check decrypted header)
			remaining := len(src) - 16
			decryptLen := min(remaining, 48)
			if decryptLen > 0 {
				decrypted := ReverseTransCodePartial(src[16 : 16+decryptLen])
				copy(dst[16:], decrypted)
			}
			if remaining > 48 {
				copy(dst[64:], src[64:])
			}
		} else { // Full decryption
			decrypted := ReverseTransCodePartial(src[16:])
			copy(dst[16:], decrypted)
		}
	}
	return dst
}

func RandRead(b []byte) {
	_, _ = rand.Read(b)
}

func swap(src, dst []byte, n int) {
	switch n {
	case 8:
		dst[0], dst[1], dst[2], dst[3] = src[7], src[4], src[3], src[2]
		dst[4], dst[5], dst[6], dst[7] = src[1], src[6], src[5], src[0]
	case 16:
		dst[0], dst[1], dst[2], dst[3] = src[11], src[9], src[8], src[15]
		dst[4], dst[5], dst[6], dst[7] = src[13], src[10], src[12], src[14]
		dst[8], dst[9], dst[10], dst[11] = src[2], src[1], src[5], src[0]
		dst[12], dst[13], dst[14], dst[15] = src[6], src[4], src[7], src[3]
	default:
		copy(dst, src[:n])
	}
}
