package creds

import (
	"net/url"
	"strings"
)

// Query parameters whose values are credentials. Cloud sources (nest, ring,
// tuya, hass, xiaomi, roborock, ...) take tokens and passwords this way:
//
//	nest:?client_id=...&client_secret=...&refresh_token=...
//
// Values that arrive through ${VAR} are registered by GetValue; values written
// literally into a source URL were not registered anywhere, so they reached
// the log, /api/log and /api/streams in clear text. Userinfo (user:pass@) is
// handled separately by the regexp in SecretString.
var secretQueryKeys = map[string]struct{}{
	"password":      {},
	"pass":          {},
	"secret":        {},
	"client_secret": {},
	"refresh_token": {},
	"access_token":  {},
	"token":         {},
	"api_key":       {},
	"key":           {},
	"auth":          {},
}

// A registered secret is replaced everywhere it appears. A short value (a
// port, a channel number, a word) would mask unrelated text in every log line,
// so anything below this length is left alone. Real tokens are far longer.
const minSecretLen = 8

// AddURLSecrets registers the credential-bearing query parameters of a source
// URL as secrets. Identifying parameters (client_id, project_id, device_id,
// host) are untouched so diagnostics stay readable: the URL logs as
// "client_secret=***&project_id=abc". Safe on any string: no query, or one that
// does not parse, is a no-op.
func AddURLSecrets(rawURL string) {
	i := strings.IndexByte(rawURL, '?')
	if i < 0 {
		return
	}
	// ParseQuery keeps parsing past a bad pair and returns what it could
	// read alongside the error. Use it: a malformed URL is exactly where a
	// secret would otherwise slip through, so never bail on the error.
	query, _ := url.ParseQuery(rawURL[i+1:])
	for key, values := range query {
		if _, ok := secretQueryKeys[strings.ToLower(key)]; !ok {
			continue
		}
		for _, value := range values {
			if len(value) >= minSecretLen {
				AddSecret(value)
			}
		}
	}
}
