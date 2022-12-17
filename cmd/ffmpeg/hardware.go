package ffmpeg

import (
	"github.com/rs/zerolog/log"
	"os/exec"
	"strings"
)

const (
	EngineSoftware     = "software"
	EngineVAAPI        = "vaapi"        // Intel iGPU and AMD GPU
	EngineV4L2         = "v4l2"         // Raspberry Pi 3 and 4
	EngineCUDA         = "cuda"         // NVidia on Windows and Linux
	EngineDXVA2        = "dxva2"        // Intel on Windows
	EngineVideoToolbox = "videotoolbox" // macOS
)

var cache = map[string]string{}

// MakeHardware converts software FFmpeg args to hardware args
// empty engine for autoselect
func MakeHardware(args *Args, engine string) {
	for i, codec := range args.codecs {
		if len(codec) < 12 {
			continue // skip short line (-c:v libx264...)
		}

		// get current codec name
		name := cut(codec, ' ', 1)
		switch name {
		case "libx264":
			name = "h264"
		case "libx265":
			name = "h265"
		case "mjpeg":
		default:
			continue // skip unsupported codec
		}

		// temporary disable probe for H265 and MJPEG
		if engine == "" && name == "h264" {
			if engine = cache[name]; engine == "" {
				engine = ProbeHardware(name)
				cache[name] = engine
			}
		}

		switch engine {
		case EngineVAAPI:
			args.input = "-hwaccel vaapi -hwaccel_output_format vaapi " + args.input
			args.codecs[i] = defaults[name+"/"+engine]

			for i, filter := range args.filters {
				if strings.HasPrefix(filter, "scale=") {
					args.filters[i] = "scale_vaapi=" + filter[6:]
				}
			}

			// fix if input doesn't support hwaccel, do nothing when support
			args.InsertFilter("format=vaapi|nv12,hwupload")

		case EngineCUDA:
			args.input = "-hwaccel cuda -hwaccel_output_format cuda -extra_hw_frames 2 " + args.input
			args.codecs[i] = defaults[name+"/"+engine]

			for i, filter := range args.filters {
				if strings.HasPrefix(filter, "scale=") {
					args.filters[i] = "scale_cuda=" + filter[6:]
				}
			}

		case EngineDXVA2:
			args.input = "-hwaccel dxva2 -hwaccel_output_format dxva2_vld " + args.input
			args.codecs[i] = defaults[name+"/"+engine]

			for i, filter := range args.filters {
				if strings.HasPrefix(filter, "scale=") {
					args.filters[i] = "scale_qsv=" + filter[6:]
				}
			}

			args.InsertFilter("hwmap=derive_device=qsv,format=qsv")

		case EngineVideoToolbox:
			args.input = "-hwaccel videotoolbox -hwaccel_output_format videotoolbox_vld " + args.input
			args.codecs[i] = defaults[name+"/"+engine]

		case EngineV4L2:
			args.codecs[i] = defaults[name+"/"+engine]
		}
	}
}

func run(arg ...string) bool {
	err := exec.Command(defaults["bin"], arg...).Run()
	log.Printf("%v %v", arg, err)
	return err == nil
}

func cut(s string, sep byte, pos int) string {
	for n := 0; n < pos; n++ {
		if i := strings.IndexByte(s, sep); i > 0 {
			s = s[i+1:]
		} else {
			return ""
		}
	}
	if i := strings.IndexByte(s, sep); i > 0 {
		return s[:i]
	}
	return s
}
