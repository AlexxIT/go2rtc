package ffmpeg

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/stretchr/testify/require"
)

func TestTranscodeSources(t *testing.T) {
	output := &core.Codec{Name: core.CodecAAC, ClockRate: 8000, Channels: 1}
	require.Equal(t,
		"ffmpeg -hide_banner -v error -f s16le -ar 16000 -ac 1 -i - -c:a aac -ar 8000 -ac 1 -f adts -flush_packets 1 -#audio=pcml/16000",
		aacSource("ffmpeg", output),
	)

	output = &core.Codec{Name: core.CodecELD, ClockRate: 16000, Channels: 1}
	require.Contains(t, eldSource("ffmpeg", output), "-c:a libfdk_aac -profile:a aac_eld -frame_size 480 -b:a 24k -ar 16000 -ac 1 -f latm")

	require.Equal(t, "", outputArgs(&core.Codec{Name: core.CodecAAC}))
}

// TestTranscodeAAC runs the AAC transcoder with the FFmpeg on this machine.
func TestTranscodeAAC(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg")
	}
	require.False(t, hasEncoder(bin, "not_an_encoder"))
	require.True(t, hasEncoder(bin, "aac"))

	source := aacSource(bin, &core.Codec{Name: core.CodecAAC, ClockRate: 16000, Channels: 1})
	cmdline, _, _ := strings.Cut(source, "#")
	cmd := shell.NewCommand(cmdline)
	cmd.Stdin = bytes.NewReader(make([]byte, 32000)) // 1 s of silence
	b, err := cmd.Output()
	require.NoError(t, err)

	prod, err := aac.Open(bytes.NewReader(b))
	require.NoError(t, err)
	codec := prod.GetMedias()[0].Codecs[0]
	require.Equal(t, core.CodecAAC, codec.Name)
	require.Equal(t, uint32(16000), codec.ClockRate)
}
