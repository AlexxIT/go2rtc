package tcp

import (
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
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
	// hexFn is the hash used for this Digest session (MD5 by default; SHA-256 /
	// SHA-512-256 per RFC 7616 when the server's challenge asks for them — e.g.
	// Hikvision-OEM firmware that offers only algorithm="SHA-256" for RTSP).
	hexFn func(...string) string
	// algo is the algorithm token echoed back in the Authorization header.
	// Empty for plain MD5 to preserve the original wire behaviour.
	algo string
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

		// Pick the hash from the challenge's algorithm token. Default MD5.
		a.hexFn, a.algo = digestHash(Between(auth, `algorithm="`, `"`))

		a.h1nonce = a.hexFn(a.user, realm, a.pass) + ":" + nonce
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
		hexFn := a.hexFn
		if hexFn == nil {
			hexFn = HexMD5
		}
		h2 := hexFn(req.Method, uri)
		response := hexFn(a.h1nonce, h2)
		header := a.header + fmt.Sprintf(
			`, uri="%s", response="%s"`, uri, response,
		)
		// Echo algorithm for non-MD5 digests (some firmware validates it);
		// omitted for MD5 to keep the original wire format byte-for-byte.
		if a.algo != "" {
			header += fmt.Sprintf(`, algorithm="%s"`, a.algo)
		}
		req.Header.Set("Authorization", header)
	case AuthTPLink:
		req.URL.Host = "127.0.0.1"
	}
}

func (a *Auth) Validate(req *Request) (valid, empty bool) {
	if a == nil {
		return true, true
	}

	header := req.Header.Get("Authorization")
	if header == "" {
		return false, true
	}

	if a.Method == AuthUnknown {
		a.Method = AuthBasic
		a.header = "Basic " + B64(a.user, a.pass)
	}

	return header == a.header, false
}

func (a *Auth) ReadNone(res *Response) bool {
	auth := res.Header.Get("WWW-Authenticate")
	if strings.Contains(auth, "TP-LINK Streaming Media") {
		a.Method = AuthTPLink
		return true
	}
	return false
}

func (a *Auth) UserInfo() *url.Userinfo {
	return url.UserPassword(a.user, a.pass)
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

// digestHash maps an RFC 7616 algorithm token to its hash function and the
// token to echo back in the Authorization header. Unknown/empty tokens fall
// back to MD5 with no echoed algorithm (original go2rtc behaviour).
func digestHash(algorithm string) (func(...string) string, string) {
	switch strings.TrimSuffix(strings.ToUpper(algorithm), "-SESS") {
	case "SHA-256":
		return HexSHA256, "SHA-256"
	case "SHA-512-256":
		return HexSHA512_256, "SHA-512-256"
	default:
		return HexMD5, ""
	}
}

func HexMD5(s ...string) string {
	b := md5.Sum([]byte(strings.Join(s, ":")))
	return hex.EncodeToString(b[:])
}

func HexSHA256(s ...string) string {
	b := sha256.Sum256([]byte(strings.Join(s, ":")))
	return hex.EncodeToString(b[:])
}

func HexSHA512_256(s ...string) string {
	b := sha512.Sum512_256([]byte(strings.Join(s, ":")))
	return hex.EncodeToString(b[:])
}

func B64(s ...string) string {
	b := []byte(strings.Join(s, ":"))
	return base64.StdEncoding.EncodeToString(b)
}
