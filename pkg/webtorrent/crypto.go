package webtorrent

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"
)

type Cipher struct {
	gcm   cipher.AEAD
	iv    []byte
	nonce []byte
}

func NewCipher(share, pwd, nonce string) (*Cipher, error) {
	timestamp, err := strconv.ParseInt(nonce, 36, 64)
	if err != nil {
		return nil, err
	}

	delta := time.Duration(time.Now().UnixNano() - timestamp)
	if delta < 0 {
		delta = -delta
	}

	// protect from replay attack, but respect wrong timezone on server
	if delta > 12*time.Hour {
		return nil, fmt.Errorf("wrong timedelta %s", delta)
	}

	c := &Cipher{}

	hash := sha256.New()
	hash.Write([]byte(nonce + ":" + pwd))
	key := hash.Sum(nil)

	hash.Reset()
	hash.Write([]byte(share + ":" + nonce))
	c.iv = hash.Sum(nil)[:12]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	c.gcm, err = cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	c.nonce = []byte(nonce)

	return c, nil
}

func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	return c.gcm.Open(nil, c.iv, ciphertext, c.nonce)
}

func (c *Cipher) Encrypt(plaintext []byte) []byte {
	return c.gcm.Seal(nil, c.iv, plaintext, c.nonce)
}

func InfoHash(share string) string {
	hash := sha256.New()
	hash.Write([]byte(share))
	sum := hash.Sum(nil)
	return base64.StdEncoding.EncodeToString(sum)
}
