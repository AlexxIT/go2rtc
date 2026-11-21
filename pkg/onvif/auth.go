package onvif

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"regexp"
	"strings"
	"time"
)

const tokenAge = 5 * time.Minute

type Auth struct {
	username string
	password string
}

func NewAuth(username, password string) *Auth {
	return &Auth{username: username, password: password}
}

func (a *Auth) Validate(b []byte) bool {
	username := FindTagValue(b, "Username")
	password := FindTagValue(b, "Password")
	if username == "" || password == "" {
		return false
	}

	var passwordType string

	re := regexp.MustCompile(`(?s)<(?:\w+:)?Password\b[^>]*\bType="([^"]+)"`)
	if m := re.FindSubmatch(b); len(m) == 2 {
		passwordType = string(m[1])
	}

	if passwordType == "" || strings.HasSuffix(passwordType, "#PasswordText") {
		return CompareHash(a.username, username)+CompareHash(a.password, password) == 2
	}

	nonce := FindTagValue(b, "Nonce")
	created := FindTagValue(b, "Created")

	if !strings.HasSuffix(passwordType, "#PasswordDigest") || nonce == "" || created == "" {
		return false
	}

	ts, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return false
	}

	now := time.Now()
	if ts.Before(now.Add(-tokenAge)) || ts.After(now.Add(tokenAge)) {
		return false
	}

	decodedNonce, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		return false
	}

	h := sha1.New()
	_, _ = h.Write(decodedNonce)
	_, _ = h.Write([]byte(created))
	_, _ = h.Write([]byte(a.password))

	digest := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return CompareHash(a.username, username)+CompareHash(digest, password) == 2
}

func CompareHash(x, y string) int {
	a := sha256.Sum256([]byte(x))
	b := sha256.Sum256([]byte(y))
	return subtle.ConstantTimeCompare(a[:], b[:])
}
