package ffmpeg

func ProbeHardware(name string) string {
	switch name {
	case "h264":
		if run("-init_hw_device", "cuda",
			"-f", "lavfi", "-i", "testsrc2", "-t", "1",
			"-c", "h264_nvenc", "-f", "null", "-") {
			return EngineCUDA
		}

		if run("-init_hw_device", "dxva2",
			"-f", "lavfi", "-i", "testsrc2", "-t", "1",
			"-c", "h264_qsv", "-f", "null", "-") {
			return EngineDXVA2
		}

	case "h265":
		if run("-init_hw_device", "cuda",
			"-f", "lavfi", "-i", "testsrc2", "-t", "1",
			"-c", "hevc_nvenc", "-f", "null", "-") {
			return EngineCUDA
		}

		if run("-init_hw_device", "dxva2",
			"-f", "lavfi", "-i", "testsrc2", "-t", "1",
			"-c", "hevc_qsv", "-f", "null", "-") {
			return EngineDXVA2
		}

	case "mjpeg":
		if run("-init_hw_device", "dxva2",
			"-f", "lavfi", "-i", "testsrc2", "-t", "1",
			"-c", "mjpeg_qsv", "-f", "null", "-") {
			return EngineDXVA2
		}
	}

	return EngineSoftware
}
