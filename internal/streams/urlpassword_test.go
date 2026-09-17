package streams

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestPasswordWithPercent documents how a camera password containing '%' has to
// be encoded by a client calling /api/frame.jpeg?src=...
//
// The value goes through TWO decoding steps on the go2rtc side:
//  1. r.URL.Query() decodes the query string
//  2. url.Parse decodes the userinfo of the resulting RTSP URL
//
// so it has to be encoded twice. Building the URL with net/url does that for free;
// string concatenation silently breaks the stream.
func TestPasswordWithPercent(t *testing.T) {
	const user, pass = "admin", "Tajne123%"
	const host, path = "192.168.1.180", "/stream1"

	// what a client MUST do
	rtspURL := (&url.URL{
		Scheme: "rtsp",
		User:   url.UserPassword(user, pass),
		Host:   host,
		Path:   path,
	}).String()

	query := url.Values{"src": []string{rtspURL}}
	req := httptest.NewRequest(http.MethodGet, "/api/frame.jpeg?"+query.Encode(), nil)

	src := req.URL.Query().Get("src")
	if src == "" {
		t.Fatal(`query param "src" was dropped`)
	}

	u, err := url.Parse(src)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", src, err)
	}

	got, _ := u.User.Password()
	if got != pass {
		t.Errorf("password = %q, want %q", got, pass)
	}
}

// TestPasswordWithPercentNaive pins the two ways this goes wrong, so nobody
// "simplifies" the client back into string concatenation.
func TestPasswordWithPercentNaive(t *testing.T) {
	t.Run("no encoding drops the param", func(t *testing.T) {
		raw := "src=rtsp://admin:Tajne123%@192.168.1.180/stream1"
		req := httptest.NewRequest(http.MethodGet, "/api/frame.jpeg?"+raw, nil)
		if src := req.URL.Query().Get("src"); src != "" {
			t.Errorf("expected src to be dropped, got %q", src)
		}
	})

	t.Run("single encoding breaks url.Parse", func(t *testing.T) {
		raw := "src=rtsp://admin:Tajne123%25@192.168.1.180/stream1"
		req := httptest.NewRequest(http.MethodGet, "/api/frame.jpeg?"+raw, nil)
		src := req.URL.Query().Get("src")
		if _, err := url.Parse(src); err == nil {
			t.Errorf("expected url.Parse(%q) to fail", src)
		}
	})
}
