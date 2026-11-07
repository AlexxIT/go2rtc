package arenti

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/yaml"
	"github.com/stretchr/testify/require"
)

func TestExtractTarget(t *testing.T) {
	tests := []struct {
		rawURL    string
		expected  string
		region    string
		server    string
		isBattery *bool
		channel   int
		stream    int
	}{
		{"arenti://camera1", "camera1", "", "", nil, 0, 1},
		{"arenti://front%20door", "front door", "", "", nil, 0, 1},
		{"arenti://ppslaa00000000000000", "ppslaa00000000000000", "", "", nil, 0, 1},
		{"arenti:///camera1", "camera1", "", "", nil, 0, 1},
		{"arenti://user:pass@/camera1", "camera1", "", "", nil, 0, 1},
		{"arenti://user:pass@host/camera1", "camera1", "", "", nil, 0, 1},
		{"arenti://camera1?country=US", "camera1", "", "", nil, 0, 1},
		{"arenti://camera1?region=eu", "camera1", "eu", "", nil, 0, 1},
		{"arenti://camera1?server=https://custom.arenti.net", "camera1", "", "https://custom.arenti.net", nil, 0, 1},
		{"arenti://camera1?battery=no", "camera1", "", "", boolPtr(false), 0, 1},
		{"arenti://camera1?battery=yes", "camera1", "", "", boolPtr(true), 0, 1},
		{"arenti://camera1?stream=hd", "camera1", "", "", nil, 0, 1},
		{"arenti://camera1?stream=sd", "camera1", "", "", nil, 0, 0},
		{"arenti://camera1?stream=1", "camera1", "", "", nil, 0, 1},
		{"arenti://camera1?stream=0", "camera1", "", "", nil, 0, 0},
		{"arenti://camera1?subtype=1", "camera1", "", "", nil, 0, 0},
		{"arenti://camera1?subtype=0", "camera1", "", "", nil, 0, 1},
		{"arenti://camera1?subtype=hd", "camera1", "", "", nil, 0, 1},
		{"arenti://camera1?subtype=sd", "camera1", "", "", nil, 0, 0},
		{"arenti://camera1?resolution=hd", "camera1", "", "", nil, 0, 1},
		{"arenti://camera1?resolution=sd", "camera1", "", "", nil, 0, 0},
		{"arenti://camera1?resolution=1", "camera1", "", "", nil, 0, 1},
		{"arenti://camera1?resolution=0", "camera1", "", "", nil, 0, 0},
		{"arenti://camera1?channel=0&stream=1", "camera1", "", "", nil, 0, 1},
		{"arenti://camera1?channel=1&stream=sd", "camera1", "", "", nil, 1, 0},
		{"arenti://camera1?channel=2&resolution=sd", "camera1", "", "", nil, 2, 0},
	}

	for _, tc := range tests {
		t.Run(tc.rawURL, func(t *testing.T) {
			_, _, _, region, server, battery, channel, stream, target, err := parseURL(tc.rawURL)
			require.NoError(t, err)
			require.Equal(t, tc.expected, target)
			require.Equal(t, tc.region, region)
			require.Equal(t, tc.server, server)
			require.Equal(t, tc.isBattery, battery)
			require.Equal(t, tc.channel, channel)
			require.Equal(t, tc.stream, stream)
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestConfigLoad(t *testing.T) {
	yamlStr := `
arenti:
  username: user@example.com
  password: "SecretPassword123!"
  country_code: US
  region: us
  server: "https://web-us.arenti.net"
  battery: false
`
	var v struct {
		Cfg map[string]any `yaml:"arenti"`
	}
	err := yaml.Unmarshal([]byte(yamlStr), &v)
	require.NoError(t, err)
	require.NotNil(t, v.Cfg)
	require.Equal(t, "user@example.com", v.Cfg["username"])
	require.Equal(t, "US", v.Cfg["country_code"])
	require.Equal(t, "us", v.Cfg["region"])
	require.Equal(t, "https://web-us.arenti.net", v.Cfg["server"])
	require.Equal(t, false, v.Cfg["battery"])
}
