package ffmpeg

import (
	"bytes"
	"errors"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/exec"
	"github.com/AlexxIT/go2rtc/cmd/ffmpeg/device"
	"github.com/AlexxIT/go2rtc/cmd/rtsp"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"net/url"
	"strconv"
	"strings"
)

func Init() {
	var cfg struct {
		Mod map[string]string `yaml:"ffmpeg"`
	}

	cfg.Mod = defaults // will be overriden from yaml

	app.LoadConfig(&cfg)

	if app.GetLogger("exec").GetLevel() >= 0 {
		defaults["global"] += " -v error"
	}

	streams.HandleFunc("ffmpeg", func(url string) (streamer.Producer, error) {
		args := parseArgs(url[7:]) // remove `ffmpeg:`
		if args == nil {
			return nil, errors.New("can't generate ffmpeg command")
		}
		return exec.Handle("exec:" + args.String())
	})

	device.Bin = defaults["bin"]
	device.Init()
}

var defaults = map[string]string{
	"bin":    "ffmpeg",
	"global": "-hide_banner",

	// inputs
	"file": "-re -stream_loop -1 -i {input}",
	"http": "-fflags nobuffer -flags low_delay -i {input}",
	"rtsp": "-fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_transport tcp -i {input}",

	// output
	"output": "-user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}",

	// `-preset superfast` - we can't use ultrafast because it doesn't support `-profile main -level 4.1`
	// `-tune zerolatency` - for minimal latency
	// `-profile high -level 4.1` - most used streaming profile
	"h264":  "-c:v libx264 -g 50 -profile:v high -level:v 4.1 -preset:v superfast -tune:v zerolatency",
	"h265":  "-c:v libx265 -g 50 -profile:v high -level:v 5.1 -preset:v superfast -tune:v zerolatency",
	"mjpeg": "-c:v mjpeg -force_duplicated_matrix:v 1 -huffman:v 0 -pix_fmt:v yuvj420p",

	"opus":       "-c:a libopus -ar:a 48000 -ac:a 2",
	"pcmu":       "-c:a pcm_mulaw -ar:a 8000 -ac:a 1",
	"pcmu/16000": "-c:a pcm_mulaw -ar:a 16000 -ac:a 1",
	"pcmu/48000": "-c:a pcm_mulaw -ar:a 48000 -ac:a 1",
	"pcma":       "-c:a pcm_alaw -ar:a 8000 -ac:a 1",
	"pcma/16000": "-c:a pcm_alaw -ar:a 16000 -ac:a 1",
	"pcma/48000": "-c:a pcm_alaw -ar:a 48000 -ac:a 1",
	"aac":        "-c:a aac", // keep sample rate and channels
	"aac/16000":  "-c:a aac -ar:a 16000 -ac:a 1",

	// hardware Intel and AMD on Linux
	// better not to set `-async_depth:v 1` like for QSV, because framedrops
	// `-bf 0` - disable B-frames is very important
	"h264/vaapi":  "-c:v h264_vaapi -g 50 -bf 0 -profile:v high -level:v 4.1 -sei:v 0",
	"h265/vaapi":  "-c:v hevc_vaapi -g 50 -bf 0 -profile:v high -level:v 5.1 -sei:v 0",
	"mjpeg/vaapi": "-c:v mjpeg_vaapi",

	// hardware Raspberry
	"h264/v4l2m2m": "-c:v h264_v4l2m2m -g 50 -bf 0",
	"h265/v4l2m2m": "-c:v hevc_v4l2m2m -g 50 -bf 0",

	// hardware NVidia on Linux and Windows
	// preset=p2 - faster, tune=ll - low latency
	"h264/cuda": "-c:v h264_nvenc -g 50 -profile:v high -level:v auto -preset:v p2 -tune:v ll",
	"h265/cuda": "-c:v hevc_nvenc -g 50 -profile:v high -level:v auto",

	// hardware Intel on Windows
	"h264/dxva2":  "-c:v h264_qsv -g 50 -bf 0 -profile:v high -level:v 4.1 -async_depth:v 1",
	"h265/dxva2":  "-c:v hevc_qsv -g 50 -bf 0 -profile:v high -level:v 5.1 -async_depth:v 1",
	"mjpeg/dxva2": "-c:v mjpeg_qsv -profile:v high -level:v 5.1",

	// hardware macOS
	"h264/videotoolbox": "-c:v h264_videotoolbox -g 50 -bf 0 -profile:v high -level:v 4.1",
	"h265/videotoolbox": "-c:v hevc_videotoolbox -g 50 -bf 0 -profile:v high -level:v 5.1",
}

func parseArgs(s string) *Args {
	// init FFmpeg arguments
	args := &Args{
		bin:    defaults["bin"],
		global: defaults["global"],
		output: defaults["output"],
	}

	var query url.Values
	if i := strings.IndexByte(s, '#'); i > 0 {
		query = parseQuery(s[i+1:])
		args.video = len(query["video"])
		args.audio = len(query["audio"])
		s = s[:i]
	}

	// Parse input:
	//   1. Input as xxxx:// link (http or rtsp or any other)
	//   2. Input as stream name
	//   3. Input as FFmpeg device (local USB camera)
	if i := strings.Index(s, "://"); i > 0 {
		switch s[:i] {
		case "http", "https", "rtmp":
			args.input = strings.Replace(defaults["http"], "{input}", s, 1)
		case "rtsp", "rtsps":
			// https://ffmpeg.org/ffmpeg-protocols.html#rtsp
			// skip unnecessary input tracks
			switch {
			case (args.video > 0 && args.audio > 0) || (args.video == 0 && args.audio == 0):
				args.input = "-allowed_media_types video+audio "
			case args.video > 0:
				args.input = "-allowed_media_types video "
			case args.audio > 0:
				args.input = "-allowed_media_types audio "
			}

			args.input += strings.Replace(defaults["rtsp"], "{input}", s, 1)
		default:
			args.input = "-i " + s
		}
	} else if streams.Get(s) != nil {
		s = "rtsp://localhost:" + rtsp.Port + "/" + s
		switch {
		case args.video > 0 && args.audio == 0:
			s += "?video"
		case args.audio > 0 && args.video == 0:
			s += "?audio"
		}
		args.input = strings.Replace(defaults["rtsp"], "{input}", s, 1)
	} else if strings.HasPrefix(s, "device?") {
		var err error
		args.input, err = device.GetInput(s)
		if err != nil {
			return nil
		}
	} else {
		args.input = strings.Replace(defaults["file"], "{input}", s, 1)
	}

	if query["async"] != nil {
		args.input = "-use_wallclock_as_timestamps 1 -async 1 " + args.input
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

		// 3. Process video codecs
		if args.video > 0 {
			for _, video := range query["video"] {
				if video != "copy" {
					args.AddCodec(defaults[video])
				} else {
					args.AddCodec("-c:v copy")
				}
			}
		} else {
			args.AddCodec("-vn")
		}

		// 4. Process audio codecs
		if args.audio > 0 {
			for _, audio := range query["audio"] {
				if audio != "copy" {
					args.AddCodec(defaults[audio])
				} else {
					args.AddCodec("-c:a copy")
				}
			}
		} else {
			args.AddCodec("-an")
		}

		if query["hardware"] != nil {
			MakeHardware(args, query["hardware"][0])
		}
	}

	if args.codecs == nil {
		args.AddCodec("-c copy")
	}

	return args
}

func parseQuery(s string) map[string][]string {
	query := map[string][]string{}
	for _, key := range strings.Split(s, "#") {
		var value string
		i := strings.IndexByte(key, '=')
		if i > 0 {
			key, value = key[:i], key[i+1:]
		}
		query[key] = append(query[key], value)
	}
	return query
}

type Args struct {
	bin     string   // ffmpeg
	global  string   // -hide_banner -v error
	input   string   // -re -stream_loop -1 -i /media/bunny.mp4
	codecs  []string // -c:v libx264 -g:v 30 -preset:v ultrafast -tune:v zerolatency
	filters []string // scale=1920:1080
	output  string   // -f rtsp {output}

	video, audio int // count of video and audio params
}

func (a *Args) AddCodec(codec string) {
	a.codecs = append(a.codecs, codec)
}

func (a *Args) AddFilter(filter string) {
	a.filters = append(a.filters, filter)
}

func (a *Args) InsertFilter(filter string) {
	a.filters = append([]string{filter}, a.filters...)
}

func (a *Args) String() string {
	b := bytes.NewBuffer(make([]byte, 0, 512))

	b.WriteString(a.bin)

	if a.global != "" {
		b.WriteByte(' ')
		b.WriteString(a.global)
	}

	b.WriteByte(' ')
	b.WriteString(a.input)

	multimode := a.video > 1 || a.audio > 1
	var iv, ia int

	for _, codec := range a.codecs {
		// support multiple video and/or audio codecs
		if multimode && len(codec) >= 5 {
			switch codec[:5] {
			case "-c:v ":
				codec = "-map 0:v:0? " + strings.ReplaceAll(codec, ":v ", ":v:"+strconv.Itoa(iv)+" ")
				iv++
			case "-c:a ":
				codec = "-map 0:a:0? " + strings.ReplaceAll(codec, ":a ", ":a:"+strconv.Itoa(ia)+" ")
				ia++
			}
		}

		b.WriteByte(' ')
		b.WriteString(codec)
	}

	if a.filters != nil {
		for i, filter := range a.filters {
			if i == 0 {
				b.WriteString(" -vf ")
			} else {
				b.WriteByte(',')
			}
			b.WriteString(filter)
		}
	}

	b.WriteByte(' ')
	b.WriteString(a.output)

	return b.String()
}
