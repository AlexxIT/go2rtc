package hardware

import (
	"github.com/AlexxIT/go2rtc/internal/api"
)

const ProbeVideoToolboxH264 = "-f lavfi -i testsrc2 -t 1 -c h264_videotoolbox -f null -"
const ProbeVideoToolboxH265 = "-f lavfi -i testsrc2 -t 1 -c hevc_videotoolbox -f null -"

func ProbeAll(bin string) []api.Stream {
	return []api.Stream{
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
		if run(bin, ProbeVideoToolboxH264) {
			return EngineVideoToolbox
		}

	case "h265":
		if run(bin, ProbeVideoToolboxH265) {
			return EngineVideoToolbox
		}
	}

	return EngineSoftware
}
