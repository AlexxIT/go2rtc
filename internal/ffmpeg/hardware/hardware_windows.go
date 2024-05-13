//go:build windows

package hardware

import "github.com/AlexxIT/go2rtc/internal/api"

const ProbeDXVA2H264 = "-init_hw_device dxva2 -f lavfi -i testsrc2 -t 1 -c h264_qsv -f null -"
const ProbeDXVA2H265 = "-init_hw_device dxva2 -f lavfi -i testsrc2 -t 1 -c hevc_qsv -f null -"
const ProbeDXVA2JPEG = "-init_hw_device dxva2 -f lavfi -i testsrc2 -t 1 -c mjpeg_qsv -f null -"
const ProbeCUDAH264 = "-init_hw_device cuda -f lavfi -i testsrc2 -t 1 -c h264_nvenc -f null -"
const ProbeCUDAH265 = "-init_hw_device cuda -f lavfi -i testsrc2 -t 1 -c hevc_nvenc -f null -"

func ProbeAll(bin string) []*api.Source {
	return []*api.Source{
		{
			Name: runToString(bin, ProbeDXVA2H264),
			URL:  "ffmpeg:...#video=h264#hardware=" + EngineDXVA2,
		},
		{
			Name: runToString(bin, ProbeDXVA2H265),
			URL:  "ffmpeg:...#video=h265#hardware=" + EngineDXVA2,
		},
		{
			Name: runToString(bin, ProbeDXVA2JPEG),
			URL:  "ffmpeg:...#video=mjpeg#hardware=" + EngineDXVA2,
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
	switch name {
	case "h264":
		if run(bin, ProbeCUDAH264) {
			return EngineCUDA
		}
		if run(bin, ProbeDXVA2H264) {
			return EngineDXVA2
		}

	case "h265":
		if run(bin, ProbeCUDAH265) {
			return EngineCUDA
		}
		if run(bin, ProbeDXVA2H265) {
			return EngineDXVA2
		}

	case "mjpeg":
		if run(bin, ProbeDXVA2JPEG) {
			return EngineDXVA2
		}
	}

	return EngineSoftware
}
