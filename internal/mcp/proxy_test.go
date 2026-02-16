package mcp

import "testing"

func TestResolveProxyURL(t *testing.T) {
	tests := []struct {
		name        string
		cliURL      string
		configProxy string
		apiListen   string
		expect      string
	}{
		{
			name:   "default",
			expect: "http://127.0.0.1:1984/mcp",
		},
		{
			name:      "cli url has priority",
			cliURL:    "http://localhost:9999/mcp",
			apiListen: ":1984",
			expect:    "http://localhost:9999/mcp",
		},
		{
			name:        "config proxy",
			configProxy: "localhost:7999",
			expect:      "http://localhost:7999/mcp",
		},
		{
			name:      "api listen with wildcard host",
			apiListen: "0.0.0.0:1888",
			expect:    "http://127.0.0.1:1888/mcp",
		},
		{
			name:      "api listen with port only",
			apiListen: ":1777",
			expect:    "http://127.0.0.1:1777/mcp",
		},
		{
			name:      "api listen with url",
			apiListen: "https://localhost:1984",
			expect:    "https://localhost:1984/mcp",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveProxyURL(tc.cliURL, tc.configProxy, tc.apiListen)
			if got != tc.expect {
				t.Fatalf("resolveProxyURL() = %q, expect %q", got, tc.expect)
			}
		})
	}
}

func TestNormalizeProxyURL(t *testing.T) {
	tests := []struct {
		in     string
		expect string
	}{
		{in: "localhost:1984", expect: "http://localhost:1984/mcp"},
		{in: "http://localhost:1984", expect: "http://localhost:1984/mcp"},
		{in: "http://localhost:1984/mcp", expect: "http://localhost:1984/mcp"},
		{in: "https://localhost:1984/", expect: "https://localhost:1984/mcp"},
	}

	for _, tc := range tests {
		got := normalizeProxyURL(tc.in)
		if got != tc.expect {
			t.Fatalf("normalizeProxyURL(%q) = %q, expect %q", tc.in, got, tc.expect)
		}
	}
}
