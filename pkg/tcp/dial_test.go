package tcp

import (
	"testing"
)

func TestFragmentParam(t *testing.T) {
	tests := []struct {
		name     string
		fragment string
		key      string
		want     string
	}{
		{
			name:     "empty fragment",
			fragment: "",
			key:      "proxy",
			want:     "",
		},
		{
			name:     "proxy without auth",
			fragment: "proxy=socks5://proxy.example.com:1080",
			key:      "proxy",
			want:     "socks5://proxy.example.com:1080",
		},
		{
			name:     "proxy with auth",
			fragment: "proxy=socks5://user:pass@proxy.example.com:1080",
			key:      "proxy",
			want:     "socks5://user:pass@proxy.example.com:1080",
		},
		{
			name:     "proxy among other params",
			fragment: "video=h264#proxy=socks5://proxy:1080#audio=aac",
			key:      "proxy",
			want:     "socks5://proxy:1080",
		},
		{
			name:     "proxy as last param",
			fragment: "video=h264#proxy=socks5://proxy:1080",
			key:      "proxy",
			want:     "socks5://proxy:1080",
		},
		{
			name:     "no matching key",
			fragment: "video=h264#audio=aac",
			key:      "proxy",
			want:     "",
		},
		{
			name:     "partial key match",
			fragment: "proxy2=socks5://proxy:1080",
			key:      "proxy",
			want:     "",
		},
		{
			name:     "other key lookup",
			fragment: "proxy=socks5://proxy:1080#video=h264",
			key:      "video",
			want:     "h264",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fragmentParam(tt.fragment, tt.key)
			if got != tt.want {
				t.Errorf("fragmentParam(%q, %q) = %q, want %q", tt.fragment, tt.key, got, tt.want)
			}
		})
	}
}
