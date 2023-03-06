package core

import (
	cryptorand "crypto/rand"
)

const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
const maxSize = byte(len(digits))

func RandString(size byte) string {
	b := make([]byte, size)
	if _, err := cryptorand.Read(b); err != nil {
		panic(err)
	}
	for i := byte(0); i < size; i++ {
		b[i] = digits[b[i]%maxSize]
	}
	return string(b)
}
