package ffmpeg

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	ex "github.com/AlexxIT/go2rtc/internal/exec"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

// Built-in transcoders for two-way audio with codecs FFmpeg can encode, used
// automatically when a camera takes none of the microphone's codecs (see
// exec transcoders). AAC uses FFmpeg's own encoder, so it works with any
// FFmpeg. AAC-ELD needs FFmpeg built with libfdk_aac, so it is offered only
// if the configured FFmpeg has that encoder.

// input is what the browser microphone is converted to for FFmpeg's stdin
const transcodeInput = "-f s16le -ar 16000 -ac 1 -i -"

func initTranscoders(bin string) {
	ex.AddTranscoder("ffmpeg/aac", &core.Codec{Name: core.CodecAAC}, func(output *core.Codec) string {
		return aacSource(bin, output)
	})

	go func() {
		if !hasEncoder(bin, "libfdk_aac") {
			return
		}
		ex.AddTranscoder("ffmpeg/eld", &core.Codec{Name: core.CodecELD}, func(output *core.Codec) string {
			return eldSource(bin, output)
		})
	}()
}

func aacSource(bin string, output *core.Codec) string {
	return fmt.Sprintf("%s -hide_banner -v error %s -c:a aac%s -f adts -flush_packets 1 -#audio=pcml/16000",
		bin, transcodeInput, outputArgs(output))
}

// eldSource encodes HomeKit AAC-ELD: 480 samples per frame, written as LOAS,
// because ADTS can't carry ELD.
func eldSource(bin string, output *core.Codec) string {
	return fmt.Sprintf("%s -hide_banner -v error %s -c:a libfdk_aac -profile:a aac_eld -frame_size 480 -b:a 24k%s -f latm -flush_packets 1 -#audio=pcml/16000",
		bin, transcodeInput, outputArgs(output))
}

func outputArgs(codec *core.Codec) (s string) {
	if codec.ClockRate != 0 {
		s += fmt.Sprintf(" -ar %d", codec.ClockRate)
	}
	if codec.Channels != 0 {
		s += fmt.Sprintf(" -ac %d", codec.Channels)
	}
	return
}

// hasEncoder checks if FFmpeg has an encoder, e.g. one that needs a
// non-default build.
func hasEncoder(bin, name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	b, err := exec.CommandContext(ctx, bin, "-hide_banner", "-encoders").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(b), " "+name+" ")
}
