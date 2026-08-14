package ffmpeg

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/stretchr/testify/require"
)

func TestBinWithSpaces(t *testing.T) {
	args := Args{
		Bin:    `C:\Program Files\Symcon\ffmpeg.exe`,
		Global: "-hide_banner",
		Input:  "-i rtsp://example.com",
		Output: "-f mjpeg -",
	}

	s := args.String()
	require.Equal(t, `"C:\Program Files\Symcon\ffmpeg.exe" -hide_banner -i rtsp://example.com -f mjpeg -`, s)

	// shell.QuoteSplit is how internal/exec parses this string back
	split := shell.QuoteSplit(s)
	require.Equal(t, `C:\Program Files\Symcon\ffmpeg.exe`, split[0])
	require.Equal(t, "-hide_banner", split[1])
}
