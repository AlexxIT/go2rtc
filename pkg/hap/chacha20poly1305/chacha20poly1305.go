package chacha20poly1305

import (
	"errors"

	"golang.org/x/crypto/chacha20poly1305"
)

var ErrInvalidParams = errors.New("chacha20poly1305: invalid params")

// Decrypt - decrypt without verify
func Decrypt(key32 []byte, nonce8 string, ciphertext []byte) ([]byte, error) {
	return DecryptAndVerify(key32, nil, []byte(nonce8), ciphertext, nil)
}

// Encrypt - encrypt without seal
func Encrypt(key32 []byte, nonce8 string, plaintext []byte) ([]byte, error) {
	return EncryptAndSeal(key32, nil, []byte(nonce8), plaintext, nil)
}

func DecryptAndVerify(key32, dst, nonce8, ciphertext, verify []byte) ([]byte, error) {
	if len(key32) != chacha20poly1305.KeySize || len(nonce8) != 8 {
		return nil, ErrInvalidParams
	}

	aead, err := chacha20poly1305.New(key32)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, chacha20poly1305.NonceSize)
	copy(nonce[4:], nonce8)

	return aead.Open(dst, nonce, ciphertext, verify)
}

func EncryptAndSeal(key32, dst, nonce8, plaintext, verify []byte) ([]byte, error) {
	if len(key32) != chacha20poly1305.KeySize || len(nonce8) != 8 {
		return nil, ErrInvalidParams
	}

	aead, err := chacha20poly1305.New(key32)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, chacha20poly1305.NonceSize)
	copy(nonce[4:], nonce8)

	return aead.Seal(dst, nonce, plaintext, verify), nil
}
