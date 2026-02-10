package yaml

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPatch(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		path   []string
		value  any
		expect string
	}{
		{
			name:   "empty config",
			src:    "",
			path:   []string{"streams", "camera1"},
			value:  "val1",
			expect: "streams:\n  camera1: val1\n",
		},
		{
			name:   "empty main key",
			src:    "#dummy",
			path:   []string{"streams", "camera1"},
			value:  "val1",
			expect: "#dummy\nstreams:\n  camera1: val1\n",
		},
		{
			name:   "single line value",
			src:    "streams:\n  camera1: url1\n  camera2: url2",
			path:   []string{"streams", "camera1"},
			value:  "val1",
			expect: "streams:\n  camera1: val1\n  camera2: url2",
		},
		{
			name:   "next line value",
			src:    "streams:\n  camera1:\n    url1\n  camera2: url2",
			path:   []string{"streams", "camera1"},
			value:  "val1",
			expect: "streams:\n  camera1: val1\n  camera2: url2",
		},
		{
			name:   "two lines value",
			src:    "streams:\n  camera1: url1\n    url2\n  camera2: url2",
			path:   []string{"streams", "camera1"},
			value:  "val1",
			expect: "streams:\n  camera1: val1\n  camera2: url2",
		},
		{
			name:   "next two lines value",
			src:    "streams:\n  camera1:\n    url1\n    url2\n  camera2: url2",
			path:   []string{"streams", "camera1"},
			value:  "val1",
			expect: "streams:\n  camera1: val1\n  camera2: url2",
		},
		{
			name:   "add array",
			src:    "",
			path:   []string{"streams", "camera1"},
			value:  []string{"val1", "val2"},
			expect: "streams:\n  camera1:\n    - val1\n    - val2\n",
		},
		{
			name:   "remove value",
			src:    "streams:\n  camera1: url1\n  camera2: url2",
			path:   []string{"streams", "camera1"},
			value:  nil,
			expect: "streams:\n  camera2: url2",
		},
		{
			name:   "add pairings",
			src:    "homekit:\n  camera1:\nstreams:\n  camera1: url1",
			path:   []string{"homekit", "camera1", "pairings"},
			value:  []string{"val1"},
			expect: "homekit:\n  camera1:\n    pairings:\n      - val1\nstreams:\n  camera1: url1",
		},
		{
			name:   "remove pairings",
			src:    "homekit:\n  camera1:\n    pairings:\n      - val1\nstreams:\n  camera1: url1",
			path:   []string{"homekit", "camera1", "pairings"},
			value:  nil,
			expect: "homekit:\n  camera1:\nstreams:\n  camera1: url1",
		},
		{
			name:   "no new line",
			src:    "streams:\n  camera1: url1",
			path:   []string{"streams", "camera1"},
			value:  "val1",
			expect: "streams:\n  camera1: val1\n",
		},
		{
			name:   "no new line",
			src:    "streams:\n  camera1: url1\nhomekit:\n  camera1:\n    name: dummy",
			path:   []string{"homekit", "camera1", "pairings"},
			value:  []string{"val1"},
			expect: "streams:\n  camera1: url1\nhomekit:\n  camera1:\n    name: dummy\n    pairings:\n      - val1\n",
		},
		{
			name:   "top-level replace",
			src:    "api:\n  listen: :1984\nlog:\n  level: trace\n",
			path:   []string{"log"},
			value:  map[string]any{"level": "debug"},
			expect: "api:\n  listen: :1984\nlog:\n  level: debug\n",
		},
		{
			name:   "top-level add",
			src:    "api:\n  listen: :1984\n",
			path:   []string{"ffmpeg"},
			value:  map[string]any{"bin": "/usr/bin/ffmpeg"},
			expect: "api:\n  listen: :1984\nffmpeg:\n  bin: /usr/bin/ffmpeg\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := Patch([]byte(tt.src), tt.path, tt.value)
			require.NoError(t, err)
			require.Equal(t, tt.expect, string(b))
		})
	}
}
