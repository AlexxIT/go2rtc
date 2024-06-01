package ffmpeg

import (
	"net/url"
	"slices"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg/device"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg/hardware"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg/virtual"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/ffmpeg"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod map[string]string `yaml:"ffmpeg"`
		Log struct {
			Level string `yaml:"ffmpeg"`
		} `yaml:"log"`
	}

	cfg.Mod = defaults // will be overriden from yaml
	cfg.Log.Level = "error"

	app.LoadConfig(&cfg)

	log = app.GetLogger("ffmpeg")

	// zerolog levels: trace debug         info warn    error fatal panic disabled
	// FFmpeg  levels: trace debug verbose info warning error fatal panic quiet
	if cfg.Log.Level == "warn" {
		cfg.Log.Level = "warning"
	}
	defaults["global"] += " -v " + cfg.Log.Level

	streams.RedirectFunc("ffmpeg", func(url string) (string, error) {
		if _, err := Version(); err != nil {
			return "", err
		}
		args := parseArgs(url[7:])
		if slices.Contains(args.Codecs, "auto") {
			return "", nil // force call streams.HandleFunc("ffmpeg")
		}
		return "exec:" + args.String(), nil
	})

	streams.HandleFunc("ffmpeg", NewProducer)

	api.HandleFunc("api/ffmpeg", apiFFmpeg)

	device.Init(defaults["bin"])
	hardware.Init(defaults["bin"])
}

var defaults = map[string]string{
	"bin":    "ffmpeg",
	"global": "-hide_banner",

	// inputs
	"file": "-re -i {input}",
	"http": "-fflags nobuffer -flags low_delay -i {input}",
	"rtsp": "-fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_flags prefer_tcp -i {input}",

	"rtsp/udp": "-fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -i {input}",

	// output
	"output":       "-user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}",
	"output/mjpeg": "-f mjpeg -",
	"output/raw":   "-f yuv4mpegpipe -",
	"output/aac":   "-f adts -",
	"output/wav":   "-f wav -",

	// `-preset superfast` - we can't use ultrafast because it doesn't support `-profile main -level 4.1`
	// `-tune zerolatency` - for minimal latency
	// `-profile high -level 4.1` - most used streaming profile
	// `-pix_fmt:v yuv420p` - important for Telegram
	"h264":  "-c:v libx264 -g 50 -profile:v high -level:v 4.1 -preset:v superfast -tune:v zerolatency -pix_fmt:v yuv420p",
	"h265":  "-c:v libx265 -g 50 -profile:v main -level:v 5.1 -preset:v superfast -tune:v zerolatency -pix_fmt:v yuv420p",
	"mjpeg": "-c:v mjpeg",
	//"mjpeg": "-c:v mjpeg -force_duplicated_matrix:v 1 -huffman:v 0 -pix_fmt:v yuvj420p",

	"raw":         "-c:v rawvideo",
	"raw/gray8":   "-c:v rawvideo -pix_fmt:v gray8",
	"raw/yuv420p": "-c:v rawvideo -pix_fmt:v yuv420p",
	"raw/yuv422p": "-c:v rawvideo -pix_fmt:v yuv422p",
	"raw/yuv444p": "-c:v rawvideo -pix_fmt:v yuv444p",

	// https://ffmpeg.org/ffmpeg-codecs.html#libopus-1
	// https://github.com/pion/webrtc/issues/1514
	// https://ffmpeg.org/ffmpeg-resampler.html
	// `-async 1` or `-min_comp 0` - force resampling for static timestamp inc, important for WebRTC audio quality
	"opus":       "-c:a libopus -application:a lowdelay -min_comp 0",
	"opus/16000": "-c:a libopus -application:a lowdelay -min_comp 0 -ar:a 16000 -ac:a 1",
	"pcmu":       "-c:a pcm_mulaw -ar:a 8000 -ac:a 1",
	"pcmu/8000":  "-c:a pcm_mulaw -ar:a 8000 -ac:a 1",
	"pcmu/16000": "-c:a pcm_mulaw -ar:a 16000 -ac:a 1",
	"pcmu/48000": "-c:a pcm_mulaw -ar:a 48000 -ac:a 1",
	"pcma":       "-c:a pcm_alaw -ar:a 8000 -ac:a 1",
	"pcma/8000":  "-c:a pcm_alaw -ar:a 8000 -ac:a 1",
	"pcma/16000": "-c:a pcm_alaw -ar:a 16000 -ac:a 1",
	"pcma/48000": "-c:a pcm_alaw -ar:a 48000 -ac:a 1",
	"aac":        "-c:a aac", // keep sample rate and channels
	"aac/16000":  "-c:a aac -ar:a 16000 -ac:a 1",
	"mp3":        "-c:a libmp3lame -q:a 8",
	"pcm":        "-c:a pcm_s16be -ar:a 8000 -ac:a 1",
	"pcm/8000":   "-c:a pcm_s16be -ar:a 8000 -ac:a 1",
	"pcm/16000":  "-c:a pcm_s16be -ar:a 16000 -ac:a 1",
	"pcm/48000":  "-c:a pcm_s16be -ar:a 48000 -ac:a 1",
	"pcml":       "-c:a pcm_s16le -ar:a 8000 -ac:a 1",
	"pcml/8000":  "-c:a pcm_s16le -ar:a 8000 -ac:a 1",
	"pcml/44100": "-c:a pcm_s16le -ar:a 44100 -ac:a 1",

	// hardware Intel and AMD on Linux
	// better not to set `-async_depth:v 1` like for QSV, because framedrops
	// `-bf 0` - disable B-frames is very important
	"h264/vaapi":  "-c:v h264_vaapi -g 50 -bf 0 -profile:v high -level:v 4.1 -sei:v 0",
	"h265/vaapi":  "-c:v hevc_vaapi -g 50 -bf 0 -profile:v main -level:v 5.1 -sei:v 0",
	"mjpeg/vaapi": "-c:v mjpeg_vaapi",

	// hardware Raspberry
	"h264/v4l2m2m": "-c:v h264_v4l2m2m -g 50 -bf 0",
	"h265/v4l2m2m": "-c:v hevc_v4l2m2m -g 50 -bf 0",

	// hardware Rockchip
	// important to use custom ffmpeg https://github.com/AlexxIT/go2rtc/issues/768
	// hevc - doesn't have a profile setting
	"h264/rkmpp": "-c:v h264_rkmpp_encoder -g 50 -bf 0 -profile:v high -level:v 4.1",
	"h265/rkmpp": "-c:v hevc_rkmpp_encoder -g 50 -bf 0 -level:v 5.1",

	// hardware NVidia on Linux and Windows
	// preset=p2 - faster, tune=ll - low latency
	"h264/cuda": "-c:v h264_nvenc -g 50 -bf 0 -profile:v high -level:v auto -preset:v p2 -tune:v ll",
	"h265/cuda": "-c:v hevc_nvenc -g 50 -bf 0 -profile:v main -level:v auto",

	// hardware Intel on Windows
	"h264/dxva2":  "-c:v h264_qsv -g 50 -bf 0 -profile:v high -level:v 4.1 -async_depth:v 1",
	"h265/dxva2":  "-c:v hevc_qsv -g 50 -bf 0 -profile:v main -level:v 5.1 -async_depth:v 1",
	"mjpeg/dxva2": "-c:v mjpeg_qsv",

	// hardware macOS
	"h264/videotoolbox": "-c:v h264_videotoolbox -g 50 -bf 0 -profile:v high -level:v 4.1",
	"h265/videotoolbox": "-c:v hevc_videotoolbox -g 50 -bf 0 -profile:v main -level:v 5.1",
}

var log zerolog.Logger

// configTemplate - return template from config (defaults) if exist or return raw template
func configTemplate(template string) string {
	if s := defaults[template]; s != "" {
		return s
	}
	return template
}

// inputTemplate - select input template from YAML config by template name
// if query has input param - select another template by this name
// if there is no another template - use input param as template
func inputTemplate(name, s string, query url.Values) string {
	var template string
	if input := query.Get("input"); input != "" {
		template = configTemplate(input)
	} else {
		template = defaults[name]
	}
	return strings.Replace(template, "{input}", s, 1)
}

func parseArgs(s string) *ffmpeg.Args {
	// init FFmpeg arguments
	args := &ffmpeg.Args{
		Bin:     defaults["bin"],
		Global:  defaults["global"],
		Output:  defaults["output"],
		Version: verAV,
	}

	var query url.Values
	if i := strings.IndexByte(s, '#'); i >= 0 {
		query = streams.ParseQuery(s[i+1:])
		args.Video = len(query["video"])
		args.Audio = len(query["audio"])
		s = s[:i]
	}

	// Parse input:
	//   1. Input as xxxx:// link (http or rtsp or any other)
	//   2. Input as stream name
	//   3. Input as FFmpeg device (local USB camera)
	if i := strings.Index(s, "://"); i > 0 {
		switch s[:i] {
		case "http", "https", "rtmp":
			args.Input = inputTemplate("http", s, query)
		case "rtsp", "rtsps":
			// https://ffmpeg.org/ffmpeg-protocols.html#rtsp
			// skip unnecessary input tracks
			switch {
			case (args.Video > 0 && args.Audio > 0) || (args.Video == 0 && args.Audio == 0):
				args.Input = "-allowed_media_types video+audio "
			case args.Video > 0:
				args.Input = "-allowed_media_types video "
			case args.Audio > 0:
				args.Input = "-allowed_media_types audio "
			}

			args.Input += inputTemplate("rtsp", s, query)
		default:
			args.Input = "-i " + s
		}
	} else if streams.Get(s) != nil {
		s = "rtsp://127.0.0.1:" + rtsp.Port + "/" + s
		switch {
		case args.Video > 0 && args.Audio == 0:
			s += "?video"
		case args.Audio > 0 && args.Video == 0:
			s += "?audio"
		default:
			s += "?video&audio"
		}
		args.Input = inputTemplate("rtsp", s, query)
	} else if i = strings.Index(s, "?"); i > 0 {
		switch s[:i] {
		case "device":
			args.Input = device.GetInput(s[i+1:])
		case "virtual":
			args.Input = virtual.GetInput(s[i+1:])
		case "tts":
			args.Input = virtual.GetInputTTS(s[i+1:])
		}
	} else {
		args.Input = inputTemplate("file", s, query)
	}

	if query["async"] != nil {
		args.Input = "-use_wallclock_as_timestamps 1 -async 1 " + args.Input
	}

	// Parse query params:
	//   1. `width`/`height` params
	//   2. `rotate` param
	//   3. `video` params (support multiple)
	//   4. `audio` params (support multiple)
	//   5. `hardware` param
	if query != nil {
		// 1. Process raw params for FFmpeg
		for _, raw := range query["raw"] {
			// support templates https://github.com/AlexxIT/go2rtc/issues/487
			raw = configTemplate(raw)
			args.AddCodec(raw)
		}

		// 2. Process video filters (resize and rotation)
		if query["width"] != nil || query["height"] != nil {
			filter := "scale="
			if query["width"] != nil {
				filter += query["width"][0]
			} else {
				filter += "-1"
			}
			filter += ":"
			if query["height"] != nil {
				filter += query["height"][0]
			} else {
				filter += "-1"
			}
			args.AddFilter(filter)
		}

		if query["rotate"] != nil {
			var filter string
			switch query["rotate"][0] {
			case "90":
				filter = "transpose=1" // 90 degrees clockwise
			case "180":
				filter = "transpose=1,transpose=1"
			case "-90", "270":
				filter = "transpose=2" // 90 degrees counterclockwise
			}
			if filter != "" {
				args.AddFilter(filter)
			}
		}

		for _, drawtext := range query["drawtext"] {
			// support templates https://github.com/AlexxIT/go2rtc/issues/487
			drawtext = configTemplate(drawtext)

			// support default timestamp format
			if !strings.Contains(drawtext, "text=") {
				drawtext += `:text='%{localtime\:%Y-%m-%d %X}'`
			}

			args.AddFilter("drawtext=" + drawtext)
		}

		// 3. Process video codecs
		if args.Video > 0 {
			for _, video := range query["video"] {
				if video != "copy" {
					if codec := defaults[video]; codec != "" {
						args.AddCodec(codec)
					} else {
						args.AddCodec(video)
					}
				} else {
					args.AddCodec("-c:v copy")
				}
			}
		}

		if query["bitrate"] != nil {
			// https://trac.ffmpeg.org/wiki/Limiting%20the%20output%20bitrate
			b := query["bitrate"][0]
			args.AddCodec("-b:v " + b + " -maxrate " + b + " -bufsize " + b)
		}

		// 4. Process audio codecs
		if args.Audio > 0 {
			for _, audio := range query["audio"] {
				if audio != "copy" {
					if codec := defaults[audio]; codec != "" {
						args.AddCodec(codec)
					} else {
						args.AddCodec(audio)
					}
				} else {
					args.AddCodec("-c:a copy")
				}
			}
		}

		if query["hardware"] != nil {
			hardware.MakeHardware(args, query["hardware"][0], defaults)
		}
	}

	switch {
	case args.Video == 0 && args.Audio == 0:
		args.AddCodec("-c copy")
	case args.Video == 0:
		args.AddCodec("-vn")
	case args.Audio == 0:
		args.AddCodec("-an")
	}

	// change otput from RTSP to some other pipe format
	switch {
	case args.Video == 0 && args.Audio == 0:
		// no transcoding from mjpeg input (ffmpeg device with support output as raw MJPEG)
		if strings.Contains(args.Input, " mjpeg ") {
			args.Output = defaults["output/mjpeg"]
		}
	case args.Video == 1 && args.Audio == 0:
		switch core.Before(query.Get("video"), "/") {
		case "mjpeg":
			args.Output = defaults["output/mjpeg"]
		case "raw":
			args.Output = defaults["output/raw"]
		}
	case args.Video == 0 && args.Audio == 1:
		switch core.Before(query.Get("audio"), "/") {
		case "aac":
			args.Output = defaults["output/aac"]
		case "pcma", "pcmu", "pcml":
			args.Output = defaults["output/wav"]
		}
	}

	return args
}
