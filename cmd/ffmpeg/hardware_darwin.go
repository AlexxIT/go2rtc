package ffmpeg

func ProbeHardware(name string) string {
	switch name {
	case "h264":
		if run(
			"-f", "lavfi", "-i", "testsrc2", "-t", "1",
			"-c", "h264_videotoolbox", "-f", "null", "-") {
			return EngineVideoToolbox
		}

	case "h265":
		if run(
			"-f", "lavfi", "-i", "testsrc2", "-t", "1",
			"-c", "hevc_videotoolbox", "-f", "null", "-") {
			return EngineVideoToolbox
		}
	}

	return EngineSoftware
}
