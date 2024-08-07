//go:build unix && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

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
	ProbeVAAPIH264   = "-init_hw_device vaapi -f lavfi -i testsrc2 -t 1 -vf format=nv12,hwupload -c h264_vaapi -f null -"
	ProbeVAAPIH265   = "-init_hw_device vaapi -f lavfi -i testsrc2 -t 1 -vf format=nv12,hwupload -c hevc_vaapi -f null -"
	ProbeVAAPIJPEG   = "-init_hw_device vaapi -f lavfi -i testsrc2 -t 1 -vf format=nv12,hwupload -c mjpeg_vaapi -f null -"
	ProbeCUDAH264    = "-init_hw_device cuda -f lavfi -i testsrc2 -t 1 -c h264_nvenc -f null -"
	ProbeCUDAH265    = "-init_hw_device cuda -f lavfi -i testsrc2 -t 1 -c hevc_nvenc -f null -"
)

func ProbeAll(bin string) []*api.Source {
	var probes []struct {
		encoder string
		cmd     string
		video   string
		engine  string
	}

	if runtime.GOARCH == "arm64" || runtime.GOARCH == "arm" {
		probes = []struct {
			encoder string
			cmd     string
			video   string
			engine  string
		}{
			{"h264_v4l2m2m", ProbeV4L2M2MH264, "h264", EngineV4L2M2M},
			{"hevc_v4l2m2m", ProbeV4L2M2MH265, "h265", EngineV4L2M2M},
			{"h264_rkmpp_encoder", ProbeRKMPPH264, "h264", EngineRKMPP},
			{"hevc_rkmpp_encoder", ProbeRKMPPH265, "h265", EngineRKMPP},
		}
	} else {
		probes = []struct {
			encoder string
			cmd     string
			video   string
			engine  string
		}{
			{"h264_vaapi", ProbeVAAPIH264, "h264", EngineVAAPI},
			{"hevc_vaapi", ProbeVAAPIH265, "h265", EngineVAAPI},
			{"mjpeg_vaapi", ProbeVAAPIJPEG, "mjpeg", EngineVAAPI},
			{"h264_nvenc", ProbeCUDAH264, "h264", EngineCUDA},
			{"hevc_nvenc", ProbeCUDAH265, "h265", EngineCUDA},
		}
	}

	var sources []*api.Source
	for _, probe := range probes {
		sources = append(sources, &api.Source{
			Name: runToString(bin, probe.cmd),
			URL:  "ffmpeg:...#video=" + probe.video + "#hardware=" + probe.engine,
		})
	}
	return sources
}

func ProbeHardware(bin, name string) string {
	var probes []struct {
		encoder string
		cmd     string
		engine  string
	}

	if runtime.GOARCH == "arm64" || runtime.GOARCH == "arm" {
		switch name {
		case "h264":
			probes = []struct {
				encoder string
				cmd     string
				engine  string
			}{
				{"h264_v4l2m2m", ProbeV4L2M2MH264, EngineV4L2M2M},
				{"h264_rkmpp_encoder", ProbeRKMPPH264, EngineRKMPP},
			}
		case "h265":
			probes = []struct {
				encoder string
				cmd     string
				engine  string
			}{
				{"hevc_v4l2m2m", ProbeV4L2M2MH265, EngineV4L2M2M},
				{"hevc_rkmpp_encoder", ProbeRKMPPH265, EngineRKMPP},
			}
		}
	} else {
		switch name {
		case "h264":
			probes = []struct {
				encoder string
				cmd     string
				engine  string
			}{
				{"h264_nvenc", ProbeCUDAH264, EngineCUDA},
				{"h264_vaapi", ProbeVAAPIH264, EngineVAAPI},
			}
		case "h265":
			probes = []struct {
				encoder string
				cmd     string
				engine  string
			}{
				{"hevc_nvenc", ProbeCUDAH265, EngineCUDA},
				{"hevc_vaapi", ProbeVAAPIH265, EngineVAAPI},
			}
		case "mjpeg":
			probes = []struct {
				encoder string
				cmd     string
				engine  string
			}{
				{"mjpeg_vaapi", ProbeVAAPIJPEG, EngineVAAPI},
			}
		}
	}

	for _, probe := range probes {
		if checkAndRun(bin, probe.encoder, probe.cmd) {
			return probe.engine
		}
	}

	return EngineSoftware
}
