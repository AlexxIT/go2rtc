package tuya

import (
	"crypto/md5"
	cryptoRand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"time"

	"golang.org/x/net/publicsuffix"
)

func EncryptPassword(password, pbKey string) (string, error) {
	// Hash password with MD5
	hasher := md5.New()
	hasher.Write([]byte(password))
	hashedPassword := hex.EncodeToString(hasher.Sum(nil))

	// Decode PEM public key
	block, _ := pem.Decode([]byte("-----BEGIN PUBLIC KEY-----\n" + pbKey + "\n-----END PUBLIC KEY-----"))
	if block == nil {
		return "", errors.New("failed to decode PEM block")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", err
	}

	rsaPubKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("not an RSA public key")
	}

	// Encrypt with RSA
	encrypted, err := rsa.EncryptPKCS1v15(cryptoRand.Reader, rsaPubKey, []byte(hashedPassword))
	if err != nil {
		return "", err
	}

	// Convert to hex string
	return hex.EncodeToString(encrypted), nil
}

func IsEmailAddress(input string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(input)
}

func CreateHTTPClientWithSession() *http.Client {
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})

	if err != nil {
		return nil
	}

	return &http.Client{
		Timeout: 30 * time.Second,
		Jar:     jar,
	}
}
