package rtmp

import (
	"net/url"
	"strings"
	"testing"
)

// parseAppStream replicates the App/Stream assignment from NewClient
// so it can be tested without a real network connection.
func parseAppStream(u *url.URL) (app, stream string) {
	if args := strings.Split(u.Path, "/"); len(args) >= 2 {
		app = args[1]
		if len(args) >= 3 {
			stream = args[2]
			if u.RawQuery != "" {
				stream += "?" + u.RawQuery
			}
		}
	}
	return
}

func TestStreamNameParsing(t *testing.T) {
	tests := []struct {
		name       string
		rawURL     string
		wantApp    string
		wantStream string
	}{
		{
			name:       "plain rtmp",
			rawURL:     "rtmp://192.168.1.1/live/camera1",
			wantApp:    "live",
			wantStream: "camera1",
		},
		{
			name:       "rtmp with query params",
			rawURL:     "rtmp://host/bcs/channel0.bcs?channel=0&stream=0&user=admin&password=pass",
			wantApp:    "bcs",
			wantStream: "channel0.bcs?channel=0&stream=0&user=admin&password=pass",
		},
		{
			name:       "proxy fragment does not leak into stream name",
			rawURL:     "rtmps://dc4-1.rtmp.t.me/s/MY_KEY#proxy=socks5://proxy.example.com:1080",
			wantApp:    "s",
			wantStream: "MY_KEY",
		},
		{
			name:       "proxy with auth does not leak into stream name",
			rawURL:     "rtmps://dc4-1.rtmp.t.me/s/MY_KEY#proxy=socks5://user:pass@proxy:1080",
			wantApp:    "s",
			wantStream: "MY_KEY",
		},
		{
			name:       "query params preserved, proxy fragment excluded",
			rawURL:     "rtmp://host/live/KEY?token=abc123#proxy=socks5://proxy:1080",
			wantApp:    "live",
			wantStream: "KEY?token=abc123",
		},
		{
			name:       "rtmps telegram format",
			rawURL:     "rtmps://xxx-x.rtmp.t.me/s/xxxxxxxxxx:xxxxxxxxxxxxxxxxxxxxxx",
			wantApp:    "s",
			wantStream: "xxxxxxxxxx:xxxxxxxxxxxxxxxxxxxxxx",
		},
		{
			name:       "youtube rtmp format",
			rawURL:     "rtmp://xxx.rtmp.youtube.com/live2/xxxx-xxxx-xxxx-xxxx-xxxx",
			wantApp:    "live2",
			wantStream: "xxxx-xxxx-xxxx-xxxx-xxxx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.rawURL)
			if err != nil {
				t.Fatalf("url.Parse(%q) error: %v", tt.rawURL, err)
			}
			gotApp, gotStream := parseAppStream(u)
			if gotApp != tt.wantApp {
				t.Errorf("App = %q, want %q", gotApp, tt.wantApp)
			}
			if gotStream != tt.wantStream {
				t.Errorf("Stream = %q, want %q", gotStream, tt.wantStream)
			}
		})
	}
}

// TestFragmentNotInURL verifies that url.Parse correctly places
// the #proxy=... part into Fragment, not RawQuery or Path.
func TestFragmentNotInURL(t *testing.T) {
	u, err := url.Parse("rtmps://dc4-1.rtmp.t.me/s/KEY#proxy=socks5://proxy:1080")
	if err != nil {
		t.Fatal(err)
	}

	if u.RawQuery != "" {
		t.Errorf("RawQuery should be empty, got %q", u.RawQuery)
	}
	if u.Fragment == "" {
		t.Errorf("Fragment should not be empty")
	}
	if !strings.HasPrefix(u.Fragment, "proxy=") {
		t.Errorf("Fragment = %q, should start with proxy=", u.Fragment)
	}
}
