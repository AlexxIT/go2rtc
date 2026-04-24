package onvif

import (
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"regexp"
	"strings"
	"time"
)

const (
	UsernameTokenPasswordDigest = "#PasswordDigest"
	UsernameTokenPasswordText   = "#PasswordText"

	DefaultUsernameTokenAge = 5 * time.Minute
)

type UsernameToken struct {
	Username     string
	Password     string
	PasswordType string
	Nonce        string
	Created      string
}

func ParseUsernameToken(b []byte) *UsernameToken {
	token := &UsernameToken{
		Username: FindTagValue(b, "Username"),
		Password: FindTagValue(b, "Password"),
		Nonce:    FindTagValue(b, "Nonce"),
		Created:  FindTagValue(b, "Created"),
	}

	re := regexp.MustCompile(`(?s)<(?:\w+:)?Password\b[^>]*\bType="([^"]+)"`)
	m := re.FindSubmatch(b)
	if len(m) == 2 {
		token.PasswordType = string(m[1])
	}

	if token.Username == "" && token.Password == "" && token.Nonce == "" && token.Created == "" {
		return nil
	}

	return token
}

func ValidateUsernameToken(b []byte, username, password string, now time.Time, maxAge time.Duration) bool {
	token := ParseUsernameToken(b)
	if token == nil || token.Username != username {
		return false
	}

	return token.Validate(password, now, maxAge)
}

func (t *UsernameToken) Validate(password string, now time.Time, maxAge time.Duration) bool {
	if t.Password == "" {
		return false
	}

	if maxAge <= 0 {
		maxAge = DefaultUsernameTokenAge
	}

	passwordType := t.PasswordType
	if passwordType == "" || strings.HasSuffix(passwordType, UsernameTokenPasswordText) {
		return subtle.ConstantTimeCompare([]byte(t.Password), []byte(password)) == 1
	}

	if !strings.HasSuffix(passwordType, UsernameTokenPasswordDigest) || t.Nonce == "" || t.Created == "" {
		return false
	}

	created, err := time.Parse(time.RFC3339Nano, t.Created)
	if err != nil {
		return false
	}

	if created.Before(now.Add(-maxAge)) || created.After(now.Add(maxAge)) {
		return false
	}

	nonce, err := base64.StdEncoding.DecodeString(t.Nonce)
	if err != nil {
		return false
	}

	h := sha1.New()
	_, _ = h.Write(nonce)
	_, _ = h.Write([]byte(t.Created))
	_, _ = h.Write([]byte(password))

	digest := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(t.Password), []byte(digest)) == 1
}
