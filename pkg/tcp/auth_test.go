package tcp

import (
	"net/textproto"
	"net/url"
	"strings"
	"testing"
)

// TestDigestAuth drives Read() -> Write() for MD5, SHA-256 and SHA-512-256
// no-qop Digest challenges. Expected responses were computed independently
// (Python hashlib) from the same inputs, so this pins the wire output rather
// than testing the implementation against itself.
func TestDigestAuth(t *testing.T) {
	const (
		user   = "admin"
		pass   = "password"
		realm  = "test-realm"
		nonce  = "abc123nonce"
		method = "DESCRIBE"
		rawURL = "rtsp://cam.example:554/Streaming/Channels/101"
	)

	tests := []struct {
		name     string
		algo     string // algorithm token in the challenge ("" => none, i.e. MD5)
		wantResp string
		wantEcho string // algorithm= echoed back in the Authorization header ("" => none)
	}{
		{"MD5 (no algorithm token)", "", "b85307547e4ed055180deebf2fb535f4", ""},
		{"SHA-256", "SHA-256", "55a4266fff78e9b046e45bfe98be496009e4e7aad59d6060b5a2504b22304dfa", "SHA-256"},
		{"SHA-512-256", "SHA-512-256", "93a7bd40530ba627c7462b05011f4afde7265ae35a1dbeec1bdb8ac57b217863", "SHA-512-256"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAuth(url.UserPassword(user, pass))

			challenge := `Digest realm="` + realm + `", nonce="` + nonce + `"`
			if tt.algo != "" {
				challenge += `, algorithm="` + tt.algo + `"`
			}
			challenge += `, stale="FALSE"`

			res := &Response{Header: textproto.MIMEHeader{}}
			res.Header.Set("WWW-Authenticate", challenge)
			if !a.Read(res) {
				t.Fatal("Read returned false for a Digest challenge")
			}
			if a.Method != AuthDigest {
				t.Fatalf("Method = %d, want AuthDigest", a.Method)
			}

			u, _ := url.Parse(rawURL)
			req := &Request{Method: method, URL: u, Header: textproto.MIMEHeader{}}
			a.Write(req)

			got := req.Header.Get("Authorization")
			if !strings.Contains(got, `response="`+tt.wantResp+`"`) {
				t.Errorf("response mismatch\n got:  %s\n want: response=%q", got, tt.wantResp)
			}
			if tt.wantEcho == "" {
				if strings.Contains(got, "algorithm=") {
					t.Errorf("did not expect algorithm= for MD5, got: %s", got)
				}
			} else if !strings.Contains(got, `algorithm="`+tt.wantEcho+`"`) {
				t.Errorf("expected algorithm=%q in header, got: %s", tt.wantEcho, got)
			}
		})
	}
}
