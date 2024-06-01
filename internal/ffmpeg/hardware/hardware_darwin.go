//go:build darwin || ios

package hardware

import (
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg/helpers"
)

const (
	ProbeVideoToolboxH264 = "-f lavfi -i testsrc2=size=svga -t 1 -c h264_videotoolbox -f null -"
	ProbeVideoToolboxH265 = "-f lavfi -i testsrc2=size=svga -t 1 -c hevc_videotoolbox -f null -"
)

func ProbeAll(bin string) []*api.Source {
	return []*api.Source{
		{
			Name: runToString(bin, ProbeVideoToolboxH264),
			URL:  "ffmpeg:...#video=h264#hardware=" + EngineVideoToolbox,
		},
		{
			Name: runToString(bin, ProbeVideoToolboxH265),
			URL:  "ffmpeg:...#video=h265#hardware=" + EngineVideoToolbox,
		},
	}
}

func ProbeHardware(bin, name string) string {
	switch name {
	case "h264":
		if helpers.IsEncoderSupported("h264_videotoolbox") {
			if run(bin, ProbeVideoToolboxH264) {
				return EngineVideoToolbox
			}
		}

	case "h265":
		if helpers.IsEncoderSupported("h265_videotoolbox") {
			if run(bin, ProbeVideoToolboxH265) {
				return EngineVideoToolbox
			}
		}
	}

	return EngineSoftware
}
