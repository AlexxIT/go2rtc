package crypto

import (
	"crypto/rand"
	"encoding/hex"

	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/nacl/box"
)

func GenerateKey() ([]byte, []byte, error) {
	public, private, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return public[:], private[:], err
}

func CalcSharedKey(devicePublicB64, clientPrivateB64 string) ([]byte, error) {
	var sharedKey, publicKey, privateKey [32]byte
	if _, err := hex.Decode(publicKey[:], []byte(devicePublicB64)); err != nil {
		return nil, err
	}
	if _, err := hex.Decode(privateKey[:], []byte(clientPrivateB64)); err != nil {
		return nil, err
	}
	box.Precompute(&sharedKey, &publicKey, &privateKey)
	return sharedKey[:], nil
}

func Encode(src, key32 []byte) ([]byte, error) {
	dst := make([]byte, len(src)+8)

	if _, err := rand.Read(dst[:8]); err != nil {
		return nil, err
	}

	nonce12 := make([]byte, 12)
	copy(nonce12[4:], dst[:8])

	c, err := chacha20.NewUnauthenticatedCipher(key32, nonce12)
	if err != nil {
		return nil, err
	}

	c.XORKeyStream(dst[8:], src)

	return dst, nil
}

func Decode(src, key32 []byte) ([]byte, error) {
	return DecodeNonce(src[8:], src[:8], key32)
}

func DecodeNonce(src, nonce8, key32 []byte) ([]byte, error) {
	nonce12 := make([]byte, 12)
	copy(nonce12[4:], nonce8)

	c, err := chacha20.NewUnauthenticatedCipher(key32, nonce12)
	if err != nil {
		return nil, err
	}

	dst := make([]byte, len(src))
	c.XORKeyStream(dst, src)

	return dst, nil
}
