package hardware

import (
	"net/http"
	"os/exec"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/pkg/ffmpeg"
)

const (
	EngineSoftware     = "software"
	EngineVAAPI        = "vaapi"        // Intel iGPU and AMD GPU
	EngineV4L2M2M      = "v4l2m2m"      // Raspberry Pi 3 and 4
	EngineCUDA         = "cuda"         // NVidia on Windows and Linux
	EngineDXVA2        = "dxva2"        // Intel on Windows
	EngineVideoToolbox = "videotoolbox" // macOS
	EngineRKMPP        = "rkmpp"        // Rockchip
)

func Init(bin string) {
	api.HandleFunc("api/ffmpeg/hardware", func(w http.ResponseWriter, r *http.Request) {
		api.ResponseSources(w, ProbeAll(bin))
	})
}

// MakeHardware converts software FFmpeg args to hardware args
// empty engine for autoselect
func MakeHardware(args *ffmpeg.Args, engine string, defaults map[string]string) {
	for i, codec := range args.Codecs {
		if len(codec) < 10 {
			continue // skip short line (-c:v mjpeg...)
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

		// temporary disable probe for H265
		if engine == "" && name != "h265" {
			if engine = cache[name]; engine == "" {
				engine = ProbeHardware(args.Bin, name)
				cache[name] = engine
			}
		}

		switch engine {
		case EngineVAAPI:
			args.Codecs[i] = defaults[name+"/"+engine]

			if !args.HasFilters("drawtext=") {
				args.Input = "-hwaccel vaapi -hwaccel_output_format vaapi -hwaccel_flags allow_profile_mismatch " + args.Input

				if name == "h264" {
					fixPixelFormat(args)
				}

				for i, filter := range args.Filters {
					if strings.HasPrefix(filter, "scale=") {
						args.Filters[i] = "scale_vaapi=" + filter[6:]
					}
					if strings.HasPrefix(filter, "transpose=") {
						if filter == "transpose=1,transpose=1" { // 180 degrees half-turn
							args.Filters[i] = "transpose_vaapi=4" // reversal
						} else {
							args.Filters[i] = "transpose_vaapi=" + filter[10:]
						}
					}
				}

				// fix if input doesn't support hwaccel, do nothing when support
				// insert as first filter before hardware scale and transpose
				args.InsertFilter("format=vaapi|nv12,hwupload")
			} else {
				// enable software pixel for drawtext, scale and transpose
				args.Input = "-hwaccel vaapi -hwaccel_output_format nv12 -hwaccel_flags allow_profile_mismatch " + args.Input

				args.AddFilter("hwupload")
			}

		case EngineCUDA:
			args.Codecs[i] = defaults[name+"/"+engine]

			// CUDA doesn't support hardware transpose
			// https://github.com/AlexxIT/go2rtc/issues/389
			if !args.HasFilters("drawtext=", "transpose=") {
				args.Input = "-hwaccel cuda -hwaccel_output_format cuda " + args.Input

				for i, filter := range args.Filters {
					if strings.HasPrefix(filter, "scale=") {
						args.Filters[i] = "scale_cuda=" + filter[6:]
					}
				}
			} else {
				args.Input = "-hwaccel cuda -hwaccel_output_format nv12 " + args.Input

				args.AddFilter("hwupload")
			}

		case EngineDXVA2:
			args.Input = "-hwaccel dxva2 -hwaccel_output_format dxva2_vld " + args.Input
			args.Codecs[i] = defaults[name+"/"+engine]

			for i, filter := range args.Filters {
				if strings.HasPrefix(filter, "scale=") {
					args.Filters[i] = "scale_qsv=" + filter[6:]
				}
			}

			args.InsertFilter("hwmap=derive_device=qsv,format=qsv")

		case EngineVideoToolbox:
			args.Input = "-hwaccel videotoolbox -hwaccel_output_format videotoolbox_vld " + args.Input
			args.Codecs[i] = defaults[name+"/"+engine]

		case EngineV4L2M2M:
			args.Codecs[i] = defaults[name+"/"+engine]

		case EngineRKMPP:
			args.Codecs[i] = defaults[name+"/"+engine]

			for j, filter := range args.Filters {
				if strings.HasPrefix(filter, "scale=") {
					args.Filters = append(args.Filters[:j], args.Filters[j+1:]...)

					width, height, _ := strings.Cut(filter[6:], ":")
					if width != "-1" {
						args.Codecs[i] += " -width " + width
					}
					if height != "-1" {
						args.Codecs[i] += " -height " + height
					}
					break
				}
			}
		}
	}
}

var cache = map[string]string{}

func run(bin string, args string) bool {
	err := exec.Command(bin, strings.Split(args, " ")...).Run()
	return err == nil
}

func runToString(bin string, args string) string {
	if run(bin, args) {
		return "OK"
	} else {
		return "ERROR"
	}
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

// fixPixelFormat:
//   - good h264 pixel: yuv420p(tv, bt709) == yuv420p (mpeg/limited/tv)
//   - bad h264 pixel: yuvj420p(pc, bt709) == yuvj420p (jpeg/full/pc)
//   - bad jpeg pixel: yuvj422p(pc, bt470bg)
func fixPixelFormat(args *ffmpeg.Args) {
	// in my tests this filters has same CPU/GPU load:
	//   - "hwupload"
	//   - "hwupload,scale_vaapi=out_color_matrix=bt709:out_range=tv"
	//   - "hwupload,scale_vaapi=out_color_matrix=bt709:out_range=tv:format=nv12"
	const fixPixFmt = "out_color_matrix=bt709:out_range=tv:format=nv12"

	for i, filter := range args.Filters {
		if strings.HasPrefix(filter, "scale=") {
			args.Filters[i] = filter + ":" + fixPixFmt
			return
		}
	}

	args.Filters = append(args.Filters, "scale="+fixPixFmt)
}
