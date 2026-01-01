package crypto

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strings"
)

const delta = 0x9e3779b9

const (
	StatusDefault byte = 1
	StatusENR16   byte = 3
	StatusENR32   byte = 6
)

func XXTEADecrypt(data, key []byte) []byte {
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
			v[p] -= mx(sum, y, z, p, e, k)
			y = v[p]
		}
		z := v[n-1]
		v[0] -= mx(sum, y, z, 0, e, k)
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

func XXTEAEncrypt(data, key []byte) []byte {
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
	var sum uint32
	z := v[n-1]

	for rounds > 0 {
		sum += delta
		e := (sum >> 2) & 3
		for p := 0; p < n-1; p++ {
			y := v[p+1]
			v[p] += mx(sum, y, z, p, e, k)
			z = v[p]
		}
		y := v[0]
		v[n-1] += mx(sum, y, z, n-1, e, k)
		z = v[n-1]
		rounds--
	}

	result := make([]byte, n*4)
	for i := range n {
		binary.LittleEndian.PutUint32(result[i*4:], v[i])
	}

	return result[:len(data)]
}

func mx(sum, y, z uint32, p int, e uint32, k []uint32) uint32 {
	return ((z>>5 ^ y<<2) + (y>>3 ^ z<<4)) ^ ((sum ^ y) + (k[(p&3)^int(e)] ^ z))
}

func GenerateChallengeResponse(challengeBytes []byte, enr string, status byte) []byte {
	var secretKey []byte

	switch status {
	case StatusDefault:
		secretKey = []byte("FFFFFFFFFFFFFFFF")
	case StatusENR16:
		if len(enr) >= 16 {
			secretKey = []byte(enr[:16])
		} else {
			secretKey = make([]byte, 16)
			copy(secretKey, enr)
		}
	case StatusENR32:
		if len(enr) >= 16 {
			firstKey := []byte(enr[:16])
			challengeBytes = XXTEADecrypt(challengeBytes, firstKey)
		}
		if len(enr) >= 32 {
			secretKey = []byte(enr[16:32])
		} else if len(enr) > 16 {
			secretKey = make([]byte, 16)
			copy(secretKey, []byte(enr[16:]))
		} else {
			secretKey = []byte("FFFFFFFFFFFFFFFF")
		}
	default:
		secretKey = []byte("FFFFFFFFFFFFFFFF")
	}

	return XXTEADecrypt(challengeBytes, secretKey)
}

func CalculateAuthKey(enr, mac string) []byte {
	data := enr + strings.ToUpper(mac)
	hash := sha256.Sum256([]byte(data))
	b64 := base64.StdEncoding.EncodeToString(hash[:6])
	b64 = strings.ReplaceAll(b64, "+", "Z")
	b64 = strings.ReplaceAll(b64, "/", "9")
	b64 = strings.ReplaceAll(b64, "=", "A")
	return []byte(b64)
}
