package ffmpeg

import (
	"runtime"
)

func ProbeHardware(name string) string {
	if runtime.GOARCH == "arm64" || runtime.GOARCH == "arm" {
		switch name {
		case "h264":
			if run(
				"-f", "lavfi", "-i", "testsrc2", "-t", "1",
				"-c", "h264_v4l2m2m", "-f", "null", "-") {
				return EngineV4L2
			}

		case "h265":
			if run(
				"-f", "lavfi", "-i", "testsrc2", "-t", "1",
				"-c", "hevc_v4l2m2m", "-f", "null", "-") {
				return EngineV4L2
			}
		}

		return EngineSoftware
	}

	switch name {
	case "h264":
		if run("-init_hw_device", "cuda",
			"-f", "lavfi", "-i", "testsrc2", "-t", "1",
			"-c", "h264_nvenc", "-f", "null", "-") {
			return EngineCUDA
		}

		if run("-init_hw_device", "vaapi",
			"-f", "lavfi", "-i", "testsrc2", "-t", "1",
			"-vf", "format=nv12,hwupload",
			"-c", "h264_vaapi", "-f", "null", "-") {
			return EngineVAAPI
		}

	case "h265":
		if run("-init_hw_device", "cuda",
			"-f", "lavfi", "-i", "testsrc2", "-t", "1",
			"-c", "hevc_nvenc", "-f", "null", "-") {
			return EngineCUDA
		}

		if run("-init_hw_device", "vaapi",
			"-f", "lavfi", "-i", "testsrc2", "-t", "1",
			"-vf", "format=nv12,hwupload",
			"-c", "hevc_vaapi", "-f", "null", "-") {
			return EngineVAAPI
		}

	case "mjpeg":
		if run("-init_hw_device", "vaapi",
			"-f", "lavfi", "-i", "testsrc2", "-t", "1",
			"-vf", "format=nv12,hwupload",
			"-c", "mjpeg_vaapi", "-f", "null", "-") {
			return EngineVAAPI
		}
	}

	return EngineSoftware
}
