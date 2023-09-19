package ed25519

import (
	"crypto/ed25519"
	"errors"
)

var ErrInvalidParams = errors.New("ed25519: invalid params")

func ValidateSignature(key, data, signature []byte) bool {
	if len(key) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize {
		return false
	}

	return ed25519.Verify(key, data, signature)
}

func Signature(key, data []byte) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, ErrInvalidParams
	}

	return ed25519.Sign(key, data), nil
}
