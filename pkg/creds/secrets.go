package creds

import (
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
)

func AddSecret(value string) {
	if value == "" {
		return
	}

	secretsMu.Lock()
	defer secretsMu.Unlock()

	if slices.Contains(secrets, value) {
		return
	}

	secrets = append(secrets, value)
	secretsReplacer = nil
}

var secrets []string
var secretsMu sync.Mutex
var secretsReplacer *strings.Replacer
var userinfoRegexp *regexp.Regexp

func getReplacer() *strings.Replacer {
	secretsMu.Lock()
	defer secretsMu.Unlock()

	if secretsReplacer == nil {
		values := slices.Clone(secrets)
		slices.SortFunc(values, func(a, b string) int {
			return len(b) - len(a)
		})

		oldnew := make([]string, 0, 2*len(values))
		for _, s := range values {
			oldnew = append(oldnew, s, "***")
		}
		secretsReplacer = strings.NewReplacer(oldnew...)
	}

	if userinfoRegexp == nil {
		userinfoRegexp = regexp.MustCompile(`://[` + userinfo + `]+@`)
	}

	return secretsReplacer
}

// Uniform Resource Identifier (URI)
// https://datatracker.ietf.org/doc/html/rfc3986
const (
	unreserved = `A-Za-z0-9-._~`
	subdelims  = `!$&'()*+,;=`
	userinfo   = unreserved + subdelims + `%:`
)

func SecretString(s string) string {
	re := getReplacer()
	s = secretUserinfo(s)
	return re.Replace(s)
}

func SecretWrite(w io.Writer, s string) (n int, err error) {
	re := getReplacer()
	s = secretUserinfo(s)
	return re.WriteString(w, s)
}

func secretUserinfo(s string) string {
	return userinfoRegexp.ReplaceAllStringFunc(s, func(match string) string {
		userinfo := match[3 : len(match)-1]
		if strings.Contains(userinfo, ":") {
			return `://***:***@`
		}
		return `://***@`
	})
}

func SecretWriter(w io.Writer) io.Writer {
	return &secretWriter{w}
}

type secretWriter struct {
	w io.Writer
}

func (s *secretWriter) Write(b []byte) (int, error) {
	return SecretWrite(s.w, string(b))
}

func SecretResponse(w http.ResponseWriter) http.ResponseWriter {
	return &secretResponse{w}
}

type secretResponse struct {
	http.ResponseWriter
}

func (s *secretResponse) Write(b []byte) (int, error) {
	return SecretWrite(s.ResponseWriter, string(b))
}
