package hkdf

import (
	"crypto/sha512"
	"io"

	"golang.org/x/crypto/hkdf"
)

func Sha512(key []byte, salt, info string) ([]byte, error) {
	r := hkdf.New(sha512.New, key, []byte(salt), []byte(info))

	buf := make([]byte, 32)
	_, err := io.ReadFull(r, buf)

	return buf, err
}
