//go:build darwin || ios

package hardware

import (
	"github.com/AlexxIT/go2rtc/internal/api"
)

const (
	ProbeVideoToolboxH264 = "-f lavfi -i testsrc2=size=svga -t 1 -c h264_videotoolbox -f null -"
	ProbeVideoToolboxH265 = "-f lavfi -i testsrc2=size=svga -t 1 -c hevc_videotoolbox -f null -"
)

func ProbeAll(bin string) []*api.Source {
	probes := []struct {
		encoder string
		cmd     string
		video   string
	}{
		{"h264_videotoolbox", ProbeVideoToolboxH264, "h264"},
		{"hevc_videotoolbox", ProbeVideoToolboxH265, "h265"},
	}

	var sources []*api.Source
	for _, probe := range probes {
		sources = append(sources, &api.Source{
			Name: runToString(bin, probe.cmd),
			URL:  "ffmpeg:...#video=" + probe.video + "#hardware=" + EngineVideoToolbox,
		})
	}
	return sources
}

func ProbeHardware(bin, name string) string {
	switch name {
	case "h264":
		if checkAndRun(bin, "h264_videotoolbox", ProbeVideoToolboxH264) {
			return EngineVideoToolbox
		}
	case "h265":
		if checkAndRun(bin, "h265_videotoolbox", ProbeVideoToolboxH265) {
			return EngineVideoToolbox
		}
	}
	return EngineSoftware
}
