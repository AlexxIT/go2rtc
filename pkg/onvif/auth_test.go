package onvif

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateUsernameTokenDigest(t *testing.T) {
	b := NewEnvelopeWithUser(url.UserPassword("admin", "pass")).Bytes()
	auth := NewAuth("admin", "pass")
	require.True(t, auth.Validate(b))
	require.False(t, NewAuth("admin", "wrong").Validate(b))
	require.False(t, NewAuth("user", "pass").Validate(b))
}

func TestValidateUsernameTokenRequiresFreshNonceAndTimestamp(t *testing.T) {
	b := []byte(`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Header><wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"><wsse:UsernameToken><wsse:Username>admin</wsse:Username><wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">digest</wsse:Password></wsse:UsernameToken></wsse:Security></s:Header><s:Body /></s:Envelope>`)
	require.False(t, NewAuth("admin", "pass").Validate(b))

	b = []byte(`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Header><wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"><wsse:UsernameToken><wsse:Username>admin</wsse:Username><wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">digest</wsse:Password><wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">bm9uY2U=</wsse:Nonce><wsu:Created xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">2000-01-01T00:00:00Z</wsu:Created></wsse:UsernameToken></wsse:Security></s:Header><s:Body /></s:Envelope>`)
	require.False(t, NewAuth("admin", "pass").Validate(b))
}
