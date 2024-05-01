//go:build freebsd || netbsd || openbsd || dragonfly

package hardware

import (
	"runtime"

	"github.com/AlexxIT/go2rtc/internal/api"
)

const (
	ProbeV4L2M2MH264 = "-f lavfi -i testsrc2 -t 1 -c h264_v4l2m2m -f null -"
	ProbeV4L2M2MH265 = "-f lavfi -i testsrc2 -t 1 -c hevc_v4l2m2m -f null -"
	ProbeRKMPPH264   = "-f lavfi -i testsrc2 -t 1 -c h264_rkmpp_encoder -f null -"
	ProbeRKMPPH265   = "-f lavfi -i testsrc2 -t 1 -c hevc_rkmpp_encoder -f null -"
)

func ProbeAll(bin string) []*api.Source {
	return []*api.Source{
		{
			Name: runToString(bin, ProbeV4L2M2MH264),
			URL:  "ffmpeg:...#video=h264#hardware=" + EngineV4L2M2M,
		},
		{
			Name: runToString(bin, ProbeV4L2M2MH265),
			URL:  "ffmpeg:...#video=h265#hardware=" + EngineV4L2M2M,
		},
		{
			Name: runToString(bin, ProbeRKMPPH264),
			URL:  "ffmpeg:...#video=h264#hardware=" + EngineRKMPP,
		},
		{
			Name: runToString(bin, ProbeRKMPPH265),
			URL:  "ffmpeg:...#video=h265#hardware=" + EngineRKMPP,
		},
	}
}

func ProbeHardware(bin, name string) string {
	if runtime.GOARCH == "arm64" || runtime.GOARCH == "arm" {
		switch name {
		case "h264":
			if run(bin, ProbeV4L2M2MH264) {
				return EngineV4L2M2M
			}
			if run(bin, ProbeRKMPPH264) {
				return EngineRKMPP
			}
		case "h265":
			if run(bin, ProbeV4L2M2MH265) {
				return EngineV4L2M2M
			}
			if run(bin, ProbeRKMPPH265) {
				return EngineRKMPP
			}
		}

		return EngineSoftware
	}

	return EngineSoftware
}
