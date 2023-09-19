package curve25519

import (
	"crypto/rand"

	"golang.org/x/crypto/curve25519"
)

func GenerateKeyPair() ([]byte, []byte) {
	var publicKey, privateKey [32]byte
	_, _ = rand.Read(privateKey[:])
	curve25519.ScalarBaseMult(&publicKey, &privateKey)
	return publicKey[:], privateKey[:]
}

func SharedSecret(privateKey, otherPublicKey []byte) ([]byte, error) {
	return curve25519.X25519(privateKey, otherPublicKey)
}
