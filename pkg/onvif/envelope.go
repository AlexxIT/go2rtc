package onvif

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Envelope struct {
	buf []byte
}

const (
	prefix1 = `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tt="http://www.onvif.org/ver10/schema" xmlns:tds="http://www.onvif.org/ver10/device/wsdl" xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
`
	prefix2 = `<s:Body>
`
	suffix = `
</s:Body>
</s:Envelope>`
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
</s:Header>
`,
		user.Username(),
		base64.StdEncoding.EncodeToString(h.Sum(nil)),
		base64.StdEncoding.EncodeToString([]byte(nonce)),
		created)
	e.Append(prefix2)
	return e
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
