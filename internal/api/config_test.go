package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestMergeYAMLPreserveCommentedStreamList(t *testing.T) {
	base := `streams:
  yard:
    - #http://1.1.1.1
    - #http://2.2.2.2
    - http://3.3.3.3
    - #http://4.4.4.4
log:
  level: trace
`
	patch := `log:
  api: debug
`

	path := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(path, []byte(base), 0o644))

	out, err := mergeYAML(path, []byte(patch))
	require.NoError(t, err)

	merged := string(out)
	require.Contains(t, merged, "#http://1.1.1.1")
	require.Contains(t, merged, "#http://2.2.2.2")
	require.Contains(t, merged, "#http://4.4.4.4")
	require.Contains(t, merged, "- http://3.3.3.3")
	require.NotContains(t, merged, "- null")
	require.Contains(t, merged, "api: debug")

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))
}

func TestMergeYAMLPreserveUnchangedComments(t *testing.T) {
	base := `api:
  username: admin
streams:
  yard:
    - #http://1.1.1.1
    - http://3.3.3.3
`
	patch := `api:
  password: secret
`

	path := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(path, []byte(base), 0o644))

	out, err := mergeYAML(path, []byte(patch))
	require.NoError(t, err)

	merged := string(out)
	require.Contains(t, merged, "username: admin")
	require.Contains(t, merged, "password: secret")
	require.Contains(t, merged, "#http://1.1.1.1")
	require.NotContains(t, merged, "- null")
}

func TestMergeYAMLPreserveCommentsAndFormattingAcrossSections(t *testing.T) {
	base := `# global config comment
api: # api section comment
  username: admin # inline username comment
streams:
  # stream comment
  yard:
    - #http://1.1.1.1
    - http://3.3.3.3
log:
  format: |
    line1
    line2
`
	patch := `api:
  password: "secret value"
ffmpeg:
  bin: /usr/bin/ffmpeg
`

	path := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(path, []byte(base), 0o644))

	out, err := mergeYAML(path, []byte(patch))
	require.NoError(t, err)

	merged := string(out)
	require.Contains(t, merged, "# global config comment")
	require.Contains(t, merged, "# api section comment")
	require.Contains(t, merged, "# inline username comment")
	require.Contains(t, merged, "# stream comment")
	require.Contains(t, merged, "#http://1.1.1.1")
	require.Contains(t, merged, "password: secret value")
	require.Contains(t, merged, "format: |")
	require.NotContains(t, merged, "- null")

	assertOrder(t, merged, "api:", "streams:", "log:", "ffmpeg:")

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))
	require.Equal(t, "admin", cfg["api"].(map[string]any)["username"])
	require.Equal(t, "secret value", cfg["api"].(map[string]any)["password"])
	require.Equal(t, "/usr/bin/ffmpeg", cfg["ffmpeg"].(map[string]any)["bin"])
}

func TestMergeYAMLPreserveQuotedValuesAndNestedStructure(t *testing.T) {
	base := `api:
  username: "admin user"
  listen: ":1984"
webrtc:
  candidates:
    - "stun:stun.l.google.com:19302"
streams:
  porch:
    - "rtsp://cam.local/stream?token=a:b"
    - #disabled source
`
	patch := `webrtc:
  ice_servers:
    - urls:
        - stun:stun.cloudflare.com:3478
`

	path := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(path, []byte(base), 0o644))

	out, err := mergeYAML(path, []byte(patch))
	require.NoError(t, err)

	merged := string(out)
	require.Contains(t, merged, `username: "admin user"`)
	require.Contains(t, merged, `listen: ":1984"`)
	require.Contains(t, merged, `"rtsp://cam.local/stream?token=a:b"`)
	require.Contains(t, merged, "#disabled source")
	require.Contains(t, merged, "ice_servers:")
	require.NotContains(t, merged, "- null")

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))
	require.Equal(t, "admin user", cfg["api"].(map[string]any)["username"])
	require.Equal(t, ":1984", cfg["api"].(map[string]any)["listen"])
	require.NotNil(t, cfg["streams"])
	require.NotNil(t, cfg["webrtc"].(map[string]any)["candidates"])
	require.NotNil(t, cfg["webrtc"].(map[string]any)["ice_servers"])
}

func TestMergeYAMLPatchLogKeepsCommentedYardEntriesInline(t *testing.T) {
	base := `api:
    listen: :1984
    read_only: false
    static_dir: www
log:
    level: trace
mcp:
    enabled: true
    http: true
    sse: true
streams:
    cam_main:
        - https://example.local/stream.m3u8
    yard:
        - #http://camera.local/disabled-source-a
        - #ffmpeg:http://camera.local/disabled-source-b#video=h264
        - ffmpeg:yard#video=mjpeg
        - #homekit://camera.local/disabled-source-c
        - homekit://camera.local/enabled-source
`
	patch := `log:
  api: debug
`

	path := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(path, []byte(base), 0o644))

	out, err := mergeYAML(path, []byte(patch))
	require.NoError(t, err)

	merged := string(out)
	require.Contains(t, merged, "    level: trace")
	require.Contains(t, merged, "    api: debug")
	require.Contains(t, merged, "        - #http://camera.local/disabled-source-a")
	require.Contains(t, merged, "        - #ffmpeg:http://camera.local/disabled-source-b#video=h264")
	require.Contains(t, merged, "        - #homekit://camera.local/disabled-source-c")
	require.NotContains(t, merged, "\n        -\n")

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))
	require.Equal(t, "debug", cfg["log"].(map[string]any)["api"])
}

func TestMergeYAMLPatchLogWithTrailingSpaces(t *testing.T) {
	// trailing spaces on "- #comment" lines could confuse node parsing
	base := "api:\n    listen: :1984\nlog:\n    level: trace\nstreams:\n    yard:\n" +
		"        - #http://192.168.88.100/long/path/to/resource  \n" +
		"        - #ffmpeg:http://192.168.88.100/path#video=h264  \n" +
		"        - ffmpeg:yard#video=mjpeg\n"

	patch := "log:\n  api: debug\n"

	path := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(path, []byte(base), 0o644))

	out, err := mergeYAML(path, []byte(patch))
	require.NoError(t, err)

	merged := string(out)
	require.Contains(t, merged, "        - #http://192.168.88.100/long/path/to/resource")
	require.Contains(t, merged, "        - #ffmpeg:http://192.168.88.100/path#video=h264")
	require.NotContains(t, merged, "\n        -\n")
}

func TestMergeYAMLPatchLogPreservesLongCommentedURLs(t *testing.T) {
	base := "api:\n" +
		"    listen: :1984\n" +
		"    read_only: false\n" +
		"    static_dir: www\n" +
		"log:\n" +
		"    level: trace\n" +
		"mcp:\n" +
		"    enabled: true\n" +
		"    http: true\n" +
		"    sse: true\n" +
		"streams:\n" +
		"    sf_i280_us101:\n" +
		"        - https://wzmedia.dot.ca.gov/D4/N280_at_JCT_101.stream/playlist.m3u8\n" +
		"    testsrc_h264:\n" +
		"        - exec:ffmpeg -hide_banner -re -f lavfi -i testsrc=size=320x240:rate=15 -c:v libx264 -preset ultrafast -tune zerolatency -profile:v baseline -pix_fmt yuv420p -crf 28 -f h264 -\n" +
		"    yard:\n" +
		"        - #http://192.168.88.100/c17d5873fa8f1ca5e0f94daa46e29343/live/files/high/index.m3u8\n" +
		"        - #ffmpeg:http://192.168.88.100/c17d5873fa8f1ca5e0f94daa46e29343/live/files/high/index.m3u8#audio=opus/16000#video=h264\n" +
		"        - ffmpeg:yard#video=mjpeg\n" +
		"        - #homekit://192.168.88.100:5001?client_id=00000000-0000-0000-0000-000000000001&client_private=0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001&device_id=00:00:00:00:00:01&device_public=0000000000000000000000000000000000000000000000000000000000000001\n" +
		"        - homekit://192.168.88.100:5001?client_id=00000000-0000-0000-0000-000000000002&client_private=0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000002&device_id=00:00:00:00:00:02&device_public=0000000000000000000000000000000000000000000000000000000000000002\n"

	patch := "log:\n  api: debug\n"

	path := filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(path, []byte(base), 0o644))

	out, err := mergeYAML(path, []byte(patch))
	require.NoError(t, err)

	merged := string(out)

	// patch applied
	require.Contains(t, merged, "    api: debug")
	require.Contains(t, merged, "    level: trace")

	// commented entries must stay on same line as dash
	require.Contains(t, merged, "        - #http://192.168.88.100/")
	require.Contains(t, merged, "        - #ffmpeg:http://192.168.88.100/")
	require.Contains(t, merged, "        - #homekit://192.168.88.100:")
	require.NotContains(t, merged, "\n        -\n")

	// non-commented entries preserved
	require.Contains(t, merged, "        - ffmpeg:yard#video=mjpeg")
	require.Contains(t, merged, "        - homekit://192.168.88.100:5001?client_id=00000000-0000-0000-0000-000000000002")

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))
	require.Equal(t, "debug", cfg["log"].(map[string]any)["api"])
}

func assertOrder(t *testing.T, s string, items ...string) {
	t.Helper()

	last := -1
	for _, item := range items {
		idx := strings.Index(s, item)
		require.NotEqualf(t, -1, idx, "expected %q in output", item)
		require.Greaterf(t, idx, last, "expected %q after previous sections", item)
		last = idx
	}
}
