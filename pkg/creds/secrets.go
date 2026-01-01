package creds

import (
	"io"
	"net/http"
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

func getReplacer() *strings.Replacer {
	secretsMu.Lock()
	defer secretsMu.Unlock()

	if secretsReplacer == nil {
		oldnew := make([]string, 0, 2*len(secrets))
		for _, s := range secrets {
			oldnew = append(oldnew, s, "***")
		}
		secretsReplacer = strings.NewReplacer(oldnew...)
	}

	return secretsReplacer
}

func SecretString(s string) string {
	re := getReplacer()
	return re.Replace(s)
}

func SecretWriter(w io.Writer) io.Writer {
	return &secretWriter{w}
}

type secretWriter struct {
	w io.Writer
}

func (s *secretWriter) Write(b []byte) (int, error) {
	re := getReplacer()
	return re.WriteString(s.w, string(b))
}

type secretResponse struct {
	w http.ResponseWriter
}

func (s *secretResponse) Header() http.Header {
	return s.w.Header()
}

func (s *secretResponse) Write(b []byte) (int, error) {
	re := getReplacer()
	return re.WriteString(s.w, string(b))
}

func (s *secretResponse) WriteHeader(statusCode int) {
	s.w.WriteHeader(statusCode)
}

func SecretResponse(w http.ResponseWriter) http.ResponseWriter {
	return &secretResponse{w}
}
