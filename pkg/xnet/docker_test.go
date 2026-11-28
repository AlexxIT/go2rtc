package xnet

import (
	"net"
	"testing"
)

func TestDockerContains(t *testing.T) {
	cases := []struct {
		ip      string
		exclude bool
	}{
		{"172.17.0.1", true},
		{"172.18.0.1", false},
		{"172.30.32.1", false},
		{"172.16.250.1", false},
		{"172.29.250.1", false},
		{"192.168.1.1", false},
	}
	for _, c := range cases {
		got := Docker.Contains(net.ParseIP(c.ip))
		if got != c.exclude {
			t.Errorf("%s: Contains=%v, want %v", c.ip, got, c.exclude)
		}
	}
}
