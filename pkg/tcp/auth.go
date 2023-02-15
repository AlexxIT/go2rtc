package tcp

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

type Auth struct {
	Method  byte
	user    string
	pass    string
	header  string
	h1nonce string
}

const (
	AuthNone byte = iota
	AuthUnknown
	AuthBasic
	AuthDigest
	AuthTPLink // https://drmnsamoliu.github.io/video.html
)

func NewAuth(user *url.Userinfo) *Auth {
	a := new(Auth)
	a.user = user.Username()
	a.pass, _ = user.Password()
	if a.user != "" {
		a.Method = AuthUnknown
	}
	return a
}

func (a *Auth) Read(res *Response) bool {
	auth := res.Header.Get("WWW-Authenticate")
	if len(auth) < 6 {
		return false
	}

	switch auth[:6] {
	case "Basic ":
		a.header = "Basic " + B64(a.user, a.pass)
		a.Method = AuthBasic
		return true
	case "Digest":
		realm := Between(auth, `realm="`, `"`)
		nonce := Between(auth, `nonce="`, `"`)

		a.h1nonce = HexMD5(a.user, realm, a.pass) + ":" + nonce
		a.header = fmt.Sprintf(
			`Digest username="%s", realm="%s", nonce="%s"`,
			a.user, realm, nonce,
		)
		a.Method = AuthDigest
		return true
	default:
		return false
	}
}

func (a *Auth) Write(req *Request) {
	if a == nil {
		return
	}

	switch a.Method {
	case AuthBasic:
		req.Header.Set("Authorization", a.header)
	case AuthDigest:
		// important to use String except RequestURL for RtspServer:
		// https://github.com/AlexxIT/go2rtc/issues/244
		uri := req.URL.String()
		h2 := HexMD5(req.Method, uri)
		response := HexMD5(a.h1nonce, h2)
		header := a.header + fmt.Sprintf(
			`, uri="%s", response="%s"`, uri, response,
		)
		req.Header.Set("Authorization", header)
	case AuthTPLink:
		req.URL.Host = "127.0.0.1"
	}
}

func (a *Auth) Validate(req *Request) bool {
	if a == nil {
		return true
	}

	header := req.Header.Get("Authorization")
	if header == "" {
		return false
	}

	if a.Method == AuthUnknown {
		a.Method = AuthBasic
		a.header = "Basic " + B64(a.user, a.pass)
	}

	return header == a.header
}

func (a *Auth) ReadNone(res *Response) bool {
	auth := res.Header.Get("WWW-Authenticate")
	if strings.Contains(auth, "TP-LINK Streaming Media") {
		a.Method = AuthTPLink
		return true
	}
	return false
}

func Between(s, sub1, sub2 string) string {
	i := strings.Index(s, sub1)
	if i < 0 {
		return ""
	}
	s = s[i+len(sub1):]
	i = strings.Index(s, sub2)
	if i < 0 {
		return ""
	}
	return s[:i]
}

func HexMD5(s ...string) string {
	b := md5.Sum([]byte(strings.Join(s, ":")))
	return hex.EncodeToString(b[:])
}

func B64(s ...string) string {
	b := []byte(strings.Join(s, ":"))
	return base64.StdEncoding.EncodeToString(b)
}
