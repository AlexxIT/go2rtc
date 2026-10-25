package tcp

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Auth struct {
	Method byte
	user   string
	pass   string
	header string // Basic-only; Digest builds Authorization per request

	algorithm string // verbatim from the winning challenge; empty = implicit MD5
	realm     string
	nonce     string
	opaque    string
	qop       string // "auth" if offered; otherwise empty (legacy no-qop formula)
	cnonce    string
	nc        uint32
	ha1       string // cached in Read, reused by every Write on this connection
}

type digestChallenge struct {
	algorithm, realm, nonce, opaque, qop string
}

const (
	AuthNone byte = iota
	AuthUnknown
	AuthBasic
	AuthDigest
	AuthTPLink // https://drmnsamoliu.github.io/video.html
)

var digestParamRE = regexp.MustCompile(`(\w+)\s*=\s*(?:"([^"]*)"|([^,\s]+))`)

func NewAuth(user *url.Userinfo) *Auth {
	a := new(Auth)
	a.user = user.Username()
	a.pass, _ = user.Password()
	if a.user != "" {
		a.Method = AuthUnknown
	}
	return a
}

func parseDigestChallenge(value string) *digestChallenge {
	if !strings.HasPrefix(value, "Digest") {
		return nil
	}
	c := &digestChallenge{}
	for _, match := range digestParamRE.FindAllStringSubmatch(value, -1) {
		key := strings.ToLower(match[1])
		val := match[2]
		if val == "" {
			val = match[3]
		}
		switch key {
		case "algorithm":
			c.algorithm = val
		case "realm":
			c.realm = val
		case "nonce":
			c.nonce = val
		case "opaque":
			c.opaque = val
		case "qop":
			c.qop = val
		}
	}
	if c.realm == "" || c.nonce == "" {
		return nil
	}
	return c
}

func algorithmRank(token string) int {
	switch {
	case token == "" || strings.EqualFold(token, "MD5"):
		return 1
	case strings.EqualFold(token, "MD5-sess"):
		return 2
	case strings.EqualFold(token, "SHA-256"):
		return 3
	case strings.EqualFold(token, "SHA-256-sess"):
		return 4
	default:
		return 0
	}
}

func algorithmBase(algorithm string) string {
	if n := len(algorithm); n >= 5 && strings.EqualFold(algorithm[n-5:], "-sess") {
		return algorithm[:n-5]
	}
	return algorithm
}

func hexHash(algorithm string, parts ...string) string {
	material := []byte(strings.Join(parts, ":"))
	if strings.EqualFold(algorithmBase(algorithm), "SHA-256") {
		sum := sha256.Sum256(material)
		return hex.EncodeToString(sum[:])
	}
	sum := md5.Sum(material)
	return hex.EncodeToString(sum[:])
}

func qopOffersAuth(qop string) bool {
	for _, opt := range strings.Split(qop, ",") {
		if strings.EqualFold(strings.TrimSpace(opt), "auth") {
			return true
		}
	}
	return false
}

func (a *Auth) Read(res *Response) bool {
	values := res.Header.Values("WWW-Authenticate")
	if len(values) == 0 {
		return false
	}

	var best *digestChallenge
	bestRank := 0
	for _, value := range values {
		c := parseDigestChallenge(value)
		if c == nil {
			continue
		}
		rank := algorithmRank(c.algorithm)
		if rank > bestRank {
			best = c
			bestRank = rank
		}
	}

	if best != nil {
		a.algorithm = best.algorithm
		a.realm = best.realm
		a.nonce = best.nonce
		a.opaque = best.opaque
		a.qop = ""
		if qopOffersAuth(best.qop) {
			a.qop = "auth"
		}
		rank := algorithmRank(a.algorithm)
		if a.qop != "" || rank == 2 || rank == 4 {
			a.cnonce = core.RandString(32, 64)
			a.nc = 0
		}
		if rank == 2 || rank == 4 {
			a.ha1 = hexHash(a.algorithm, hexHash(a.algorithm, a.user, a.realm, a.pass), a.nonce, a.cnonce)
		} else {
			a.ha1 = hexHash(a.algorithm, a.user, a.realm, a.pass)
		}
		a.Method = AuthDigest
		return true
	}

	for _, value := range values {
		if strings.HasPrefix(value, "Basic") {
			a.header = "Basic " + B64(a.user, a.pass)
			a.Method = AuthBasic
			return true
		}
	}
	return false
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
		h2 := hexHash(a.algorithm, req.Method, uri)
		var header string
		if a.qop == "" {
			response := hexHash(a.algorithm, a.ha1, a.nonce, h2)
			header = fmt.Sprintf(
				`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
				a.user, a.realm, a.nonce, uri, response,
			)
			if a.algorithm != "" {
				header += ", algorithm=" + a.algorithm
			}
			if a.opaque != "" {
				header += fmt.Sprintf(`, opaque="%s"`, a.opaque)
			}
		} else {
			a.nc++
			nc := fmt.Sprintf("%08x", a.nc)
			response := hexHash(a.algorithm, a.ha1, a.nonce, nc, a.cnonce, a.qop, h2)
			header = fmt.Sprintf(
				`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=%s, qop=auth, nc=%s, cnonce="%s", response="%s"`,
				a.user, a.realm, a.nonce, uri, a.algorithm, nc, a.cnonce, response,
			)
			if a.opaque != "" {
				header += fmt.Sprintf(`, opaque="%s"`, a.opaque)
			}
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

func HexMD5(s ...string) string {
	b := md5.Sum([]byte(strings.Join(s, ":")))
	return hex.EncodeToString(b[:])
}

func B64(s ...string) string {
	b := []byte(strings.Join(s, ":"))
	return base64.StdEncoding.EncodeToString(b)
}
