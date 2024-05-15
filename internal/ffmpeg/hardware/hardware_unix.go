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
	if runtime.GOARCH == "arm64" || runtime.GOARCH == "arm" {
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

	return []*api.Source{
		{
			Name: runToString(bin, ProbeVAAPIH264),
			URL:  "ffmpeg:...#video=h264#hardware=" + EngineVAAPI,
		},
		{
			Name: runToString(bin, ProbeVAAPIH265),
			URL:  "ffmpeg:...#video=h265#hardware=" + EngineVAAPI,
		},
		{
			Name: runToString(bin, ProbeVAAPIJPEG),
			URL:  "ffmpeg:...#video=mjpeg#hardware=" + EngineVAAPI,
		},
		{
			Name: runToString(bin, ProbeCUDAH264),
			URL:  "ffmpeg:...#video=h264#hardware=" + EngineCUDA,
		},
		{
			Name: runToString(bin, ProbeCUDAH265),
			URL:  "ffmpeg:...#video=h265#hardware=" + EngineCUDA,
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

	switch name {
	case "h264":
		if run(bin, ProbeCUDAH264) {
			return EngineCUDA
		}
		if run(bin, ProbeVAAPIH264) {
			return EngineVAAPI
		}

	case "h265":
		if run(bin, ProbeCUDAH265) {
			return EngineCUDA
		}
		if run(bin, ProbeVAAPIH265) {
			return EngineVAAPI
		}

	case "mjpeg":
		if run(bin, ProbeVAAPIJPEG) {
			return EngineVAAPI
		}
	}

	return EngineSoftware
}
