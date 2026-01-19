package tutk

import (
	"encoding/binary"
	"math/bits"
)

// I'd like to say hello to Charlie. Your name is forever etched into the history of streaming software.
const charlie = "Charlie is the designer of P2P!!"

func ReverseTransCodePartial(dst, src []byte) []byte {
	n := len(src)
	tmp := make([]byte, n)
	if len(dst) < n {
		dst = make([]byte, n)
	}

	src16 := src
	tmp16 := tmp
	dst16 := dst

	for ; n >= 16; n -= 16 {
		for i := 0; i != 16; i += 4 {
			x := binary.LittleEndian.Uint32(src16[i:])
			binary.LittleEndian.PutUint32(tmp16[i:], bits.RotateLeft32(x, i+3))
		}

		swap(dst16, tmp16, 16)

		for i := 0; i != 16; i++ {
			tmp16[i] = dst16[i] ^ charlie[i]
		}

		for i := 0; i != 16; i += 4 {
			x := binary.LittleEndian.Uint32(tmp16[i:])
			binary.LittleEndian.PutUint32(dst16[i:], bits.RotateLeft32(x, i+1))
		}

		tmp16 = tmp16[16:]
		dst16 = dst16[16:]
		src16 = src16[16:]
	}

	swap(tmp16, src16, n)

	for i := 0; i < n; i++ {
		dst16[i] = tmp16[i] ^ charlie[i]
	}

	return dst
}

func ReverseTransCodeBlob(src []byte) []byte {
	if len(src) < 16 {
		return ReverseTransCodePartial(nil, src)
	}

	dst := make([]byte, len(src))
	header := ReverseTransCodePartial(nil, src[:16])
	copy(dst, header)

	if len(src) > 16 {
		if dst[3]&1 != 0 { // Partial encryption (check decrypted header)
			remaining := len(src) - 16
			decryptLen := min(remaining, 48)
			if decryptLen > 0 {
				decrypted := ReverseTransCodePartial(nil, src[16:16+decryptLen])
				copy(dst[16:], decrypted)
			}
			if remaining > 48 {
				copy(dst[64:], src[64:])
			}
		} else { // Full decryption
			decrypted := ReverseTransCodePartial(nil, src[16:])
			copy(dst[16:], decrypted)
		}
	}
	return dst
}

func TransCodePartial(dst, src []byte) []byte {
	n := len(src)
	tmp := make([]byte, n)
	if len(dst) < n {
		dst = make([]byte, n)
	}

	src16 := src
	tmp16 := tmp
	dst16 := dst

	for ; n >= 16; n -= 16 {
		for i := 0; i != 16; i += 4 {
			x := binary.LittleEndian.Uint32(src16[i:])
			binary.LittleEndian.PutUint32(tmp16[i:], bits.RotateLeft32(x, -i-1))
		}

		for i := 0; i != 16; i++ {
			dst16[i] = tmp16[i] ^ charlie[i]
		}

		swap(tmp16, dst16, 16)

		for i := 0; i != 16; i += 4 {
			x := binary.LittleEndian.Uint32(tmp16[i:])
			binary.LittleEndian.PutUint32(dst16[i:], bits.RotateLeft32(x, -i-3))
		}

		tmp16 = tmp16[16:]
		dst16 = dst16[16:]
		src16 = src16[16:]
	}

	for i := 0; i < n; i++ {
		tmp16[i] = src16[i] ^ charlie[i]
	}

	swap(dst16, tmp16, n)

	return dst
}

func TransCodeBlob(src []byte) []byte {
	if len(src) < 16 {
		return TransCodePartial(nil, src)
	}

	dst := make([]byte, len(src))
	header := TransCodePartial(nil, src[:16])
	copy(dst, header)

	if len(src) > 16 {
		if src[3]&1 != 0 { // Partial encryption
			remaining := len(src) - 16
			encryptLen := min(remaining, 48)
			if encryptLen > 0 {
				encrypted := TransCodePartial(nil, src[16:16+encryptLen])
				copy(dst[16:], encrypted)
			}
			if remaining > 48 {
				copy(dst[64:], src[64:])
			}
		} else { // Full encryption
			encrypted := TransCodePartial(nil, src[16:])
			copy(dst[16:], encrypted)
		}
	}
	return dst
}

func swap(dst, src []byte, n int) {
	switch n {
	case 2:
		_, _ = src[1], dst[1]
		dst[0] = src[1]
		dst[1] = src[0]
		return
	case 4:
		_, _ = src[3], dst[3]
		dst[0] = src[2]
		dst[1] = src[3]
		dst[2] = src[0]
		dst[3] = src[1]
		return
	case 8:
		_, _ = src[7], dst[7]
		dst[0] = src[7]
		dst[1] = src[4]
		dst[2] = src[3]
		dst[3] = src[2]
		dst[4] = src[1]
		dst[5] = src[6]
		dst[6] = src[5]
		dst[7] = src[0]
		return
	case 16:
		_, _ = src[15], dst[15]
		dst[0] = src[11]
		dst[1] = src[9]
		dst[2] = src[8]
		dst[3] = src[15]
		dst[4] = src[13]
		dst[5] = src[10]
		dst[6] = src[12]
		dst[7] = src[14]
		dst[8] = src[2]
		dst[9] = src[1]
		dst[10] = src[5]
		dst[11] = src[0]
		dst[12] = src[6]
		dst[13] = src[4]
		dst[14] = src[7]
		dst[15] = src[3]
		return
	}
	copy(dst, src[:n])
}

const delta = 0x9e3779b9

func XXTEADecrypt(dst, src, key []byte) {
	const n = int8(4) // support only 16 bytes src

	var w, k [n]uint32
	for i := int8(0); i < n; i++ {
		w[i] = binary.LittleEndian.Uint32(src)
		k[i] = binary.LittleEndian.Uint32(key)
		src = src[4:]
		key = key[4:]
	}

	rounds := 52/n + 6
	sum := uint32(rounds) * delta
	for ; rounds > 0; rounds-- {
		w0 := w[0]
		i2 := int8((sum >> 2) & 3)
		for i := n - 1; i >= 0; i-- {
			wi := w[(i-1)&3]
			ki := k[i^i2]
			t1 := (w0 ^ sum) + (wi ^ ki)
			t2 := (wi >> 5) ^ (w0 << 2)
			t3 := (w0 >> 3) ^ (wi << 4)
			w[i] -= t1 ^ (t2 + t3)
			w0 = w[i]
		}
		sum -= delta
	}

	for _, i := range w {
		binary.LittleEndian.PutUint32(dst, i)
		dst = dst[4:]
	}
}

func XXTEADecryptVar(data, key []byte) []byte {
	if len(data) < 8 || len(key) < 16 {
		return nil
	}

	k := make([]uint32, 4)
	for i := range 4 {
		k[i] = binary.LittleEndian.Uint32(key[i*4:])
	}

	n := max(len(data)/4, 2)
	v := make([]uint32, n)
	for i := 0; i < len(data)/4; i++ {
		v[i] = binary.LittleEndian.Uint32(data[i*4:])
	}

	rounds := 6 + 52/n
	sum := uint32(rounds) * delta
	y := v[0]

	for rounds > 0 {
		e := (sum >> 2) & 3
		for p := n - 1; p > 0; p-- {
			z := v[p-1]
			v[p] -= xxteaMX(sum, y, z, p, e, k)
			y = v[p]
		}
		z := v[n-1]
		v[0] -= xxteaMX(sum, y, z, 0, e, k)
		y = v[0]
		sum -= delta
		rounds--
	}

	result := make([]byte, n*4)
	for i := range n {
		binary.LittleEndian.PutUint32(result[i*4:], v[i])
	}

	return result[:len(data)]
}

func xxteaMX(sum, y, z uint32, p int, e uint32, k []uint32) uint32 {
	return ((z>>5 ^ y<<2) + (y>>3 ^ z<<4)) ^ ((sum ^ y) + (k[(p&3)^int(e)] ^ z))
}
