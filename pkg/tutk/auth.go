package tutk

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

func CalculateAuthKey(enr, mac string) []byte {
	data := enr + strings.ToUpper(mac)
	hash := sha256.Sum256([]byte(data))
	b64 := base64.StdEncoding.EncodeToString(hash[:6])
	b64 = strings.ReplaceAll(b64, "+", "Z")
	b64 = strings.ReplaceAll(b64, "/", "9")
	b64 = strings.ReplaceAll(b64, "=", "A")
	return []byte(b64)
}

func DerivePSK(enr string) []byte {
	// DerivePSK derives the DTLS PSK from ENR
	// TUTK SDK treats the PSK as a NULL-terminated C string, so if SHA256(ENR)
	// contains a 0x00 byte, the PSK is truncated at that position.
	hash := sha256.Sum256([]byte(enr))
	pskLen := 32
	for i := range 32 {
		if hash[i] == 0x00 {
			pskLen = i
			break
		}
	}

	psk := make([]byte, 32)
	copy(psk[:pskLen], hash[:pskLen])
	return psk
}
