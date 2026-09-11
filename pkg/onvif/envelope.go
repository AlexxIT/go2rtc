package onvif

import (
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Envelope struct {
	buf []byte
}

const (
	prefix1 = `<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tt="http://www.onvif.org/ver10/schema" xmlns:tds="http://www.onvif.org/ver10/device/wsdl" xmlns:trt="http://www.onvif.org/ver10/media/wsdl" xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl" xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl">`
	prefix2 = `<s:Body>`
	suffix  = `</s:Body></s:Envelope>`
)

func NewEnvelope() *Envelope {
	e := &Envelope{buf: make([]byte, 0, 1024)}
	e.Append(prefix1, prefix2)
	return e
}

func NewEnvelopeWithUser(user *url.Userinfo) *Envelope {
	if user == nil {
		return NewEnvelope()
	}

	nonce := core.RandString(16, 36)
	created := time.Now().UTC().Format(time.RFC3339Nano)
	pass, _ := user.Password()

	h := sha1.New()
	h.Write([]byte(nonce + created + pass))

	e := &Envelope{buf: make([]byte, 0, 1024)}
	e.Append(prefix1)
	e.Appendf(`<s:Header>
	<wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
		<wsse:UsernameToken>
			<wsse:Username>%s</wsse:Username>
			<wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">%s</wsse:Password>
			<wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">%s</wsse:Nonce>
			<wsu:Created xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">%s</wsu:Created>
		</wsse:UsernameToken>
	</wsse:Security>
</s:Header>`,
		user.Username(),
		base64.StdEncoding.EncodeToString(h.Sum(nil)),
		base64.StdEncoding.EncodeToString([]byte(nonce)),
		created)
	e.Append(prefix2)
	return e
}

// nonceTTL bounds both how stale a Created timestamp may be and how long a
// (username, nonce) pair is remembered for replay rejection.
const nonceTTL = 5 * time.Minute

// futureSkew is the only leeway given to a Created timestamp that's ahead of
// our clock (normal clock drift); tokens claiming to be further in the
// future than this are rejected outright rather than accepted into the
// full nonceTTL window.
const futureSkew = 30 * time.Second

var (
	seenNoncesMu sync.Mutex
	seenNonces   = map[string]time.Time{}
)

func VerifyUsernameToken(b []byte, username, password string) bool {
	if FindTagValue(b, "Username") != username {
		return false
	}

	created := FindTagValue(b, "Created")
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return false
	}
	if age := time.Since(t); age < -futureSkew || age > nonceTTL {
		return false
	}

	nonceB64 := FindTagValue(b, "Nonce")
	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return false
	}

	provided, err := base64.StdEncoding.DecodeString(FindTagValue(b, "Password"))
	if err != nil {
		return false
	}

	h := sha1.New()
	h.Write(nonce)
	h.Write([]byte(created))
	h.Write([]byte(password))
	digest := h.Sum(nil)

	if subtle.ConstantTimeCompare(digest, provided) != 1 {
		return false
	}

	// Reject replays of a previously-seen (username, nonce, created) triple.
	// ponytail: in-memory map, so restarting the process resets it; fine for
	// a single-instance server, add a shared store if you ever run more than one.
	key := username + "\x00" + nonceB64 + "\x00" + created

	seenNoncesMu.Lock()
	defer seenNoncesMu.Unlock()

	if _, dup := seenNonces[key]; dup {
		return false
	}

	now := time.Now()
	for k, exp := range seenNonces {
		if now.After(exp) {
			delete(seenNonces, k)
		}
	}
	seenNonces[key] = now.Add(nonceTTL)

	return true
}

func (e *Envelope) Append(args ...string) {
	for _, s := range args {
		e.buf = append(e.buf, s...)
	}
}

func (e *Envelope) Appendf(format string, args ...any) {
	e.buf = fmt.Appendf(e.buf, format, args...)
}

func (e *Envelope) Bytes() []byte {
	return append(e.buf, suffix...)
}
