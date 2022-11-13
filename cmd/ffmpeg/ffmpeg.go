package ffmpeg

import (
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

	// defaults

	cfg.Mod = map[string]string{
		"bin": "ffmpeg",

		// inputs
		"file": "-re -stream_loop -1 -i {input}",
		"http": "-fflags nobuffer -flags low_delay -i {input}",
		"rtsp": "-fflags nobuffer -flags low_delay -timeout 5000000 -user_agent go2rtc/ffmpeg -rtsp_transport tcp -i {input}",

		// output
		"output": "-user_agent ffmpeg/go2rtc -rtsp_transport tcp -f rtsp {output}",

		// `-g 30` - group of picture, GOP, keyframe interval
		// `-preset superfast` - we can't use ultrafast because it doesn't support `-profile main -level 4.1`
		// `-tune zerolatency` - for minimal latency
		// `-profile main -level 4.1` - most used streaming profile
		// `-pix_fmt yuv420p` - if input pix format 4:2:2
		"h264":       "-c:v libx264 -g:v 30 -preset:v superfast -tune:v zerolatency -profile:v main -level:v 4.1 -pix_fmt:v yuv420p",
		"h264/ultra": "-c:v libx264 -g:v 30 -preset:v ultrafast -tune:v zerolatency",
		"h264/high":  "-c:v libx264 -g:v 30 -preset:v superfast -tune:v zerolatency",
		"h265":       "-c:v libx265 -g:v 30 -preset:v ultrafast -tune:v zerolatency",
		"mjpeg":      "-c:v mjpeg -force_duplicated_matrix:v 1 -huffman:v 0 -pix_fmt:v yuvj420p",
		"opus":       "-c:a libopus -ar:a 48000 -ac:a 2",
		"pcmu":       "-c:a pcm_mulaw -ar:a 8000 -ac:a 1",
		"pcmu/16000": "-c:a pcm_mulaw -ar:a 16000 -ac:a 1",
		"pcmu/48000": "-c:a pcm_mulaw -ar:a 48000 -ac:a 1",
		"pcma":       "-c:a pcm_alaw -ar:a 8000 -ac:a 1",
		"pcma/16000": "-c:a pcm_alaw -ar:a 16000 -ac:a 1",
		"pcma/48000": "-c:a pcm_alaw -ar:a 48000 -ac:a 1",
		"aac":        "-c:a aac", // keep sample rate and channels
		"aac/16000":  "-c:a aac -ar:a 16000 -ac:a 1",
	}

	app.LoadConfig(&cfg)

	tpl := cfg.Mod

	cmd := "exec:" + tpl["bin"] + " -hide_banner "

	if app.GetLogger("exec").GetLevel() >= 0 {
		cmd += "-v error "
	}

	streams.HandleFunc("ffmpeg", func(s string) (streamer.Producer, error) {
		s = s[7:] // remove `ffmpeg:`

		var query url.Values
		var queryVideo, queryAudio bool

		if i := strings.IndexByte(s, '#'); i > 0 {
			query = parseQuery(s[i+1:])
			queryVideo = query["video"] != nil
			queryAudio = query["audio"] != nil
			s = s[:i]
		} else {
			// by default query both video and audio
			queryVideo = true
			queryAudio = true
		}

		var input string
		if i := strings.Index(s, "://"); i > 0 {
			switch s[:i] {
			case "http", "https", "rtmp":
				input = strings.Replace(tpl["http"], "{input}", s, 1)
			case "rtsp", "rtsps":
				// https://ffmpeg.org/ffmpeg-protocols.html#rtsp
				// skip unnecessary input tracks
				switch {
				case queryVideo && queryAudio:
					input = "-allowed_media_types video+audio "
				case queryVideo:
					input = "-allowed_media_types video "
				case queryAudio:
					input = "-allowed_media_types audio "
				}

				input += strings.Replace(tpl["rtsp"], "{input}", s, 1)
			default:
				input = "-i " + s
			}
		} else if streams.Get(s) != nil {
			s = "rtsp://localhost:" + rtsp.Port + "/" + s
			switch {
			case queryVideo && !queryAudio:
				s += "?video"
			case queryAudio && !queryVideo:
				s += "?audio"
			}
			input = strings.Replace(tpl["rtsp"], "{input}", s, 1)
		} else if strings.HasPrefix(s, "device?") {
			var err error
			input, err = device.GetInput(s)
			if err != nil {
				return nil, err
			}
		} else {
			input = strings.Replace(tpl["file"], "{input}", s, 1)
		}

		if _, ok := query["async"]; ok {
			input = "-use_wallclock_as_timestamps 1 -async 1 " + input
		}

		s = cmd + input

		if query != nil {
			for _, raw := range query["raw"] {
				s += " " + raw
			}

			for _, rotate := range query["rotate"] {
				switch rotate {
				case "90":
					s += " -vf transpose=1" // 90 degrees clockwise
				case "180":
					s += " -vf transpose=1,transpose=1"
				case "-90", "270":
					s += " -vf transpose=2" // 90 degrees counterclockwise
				}
				break
			}

			switch len(query["video"]) {
			case 0:
				s += " -vn"
			case 1:
				if len(query["audio"]) > 1 {
					s += " -map 0:v:0"
				}
				for _, video := range query["video"] {
					if video == "copy" {
						s += " -c:v copy"
					} else {
						s += " " + tpl[video]
					}
				}
			default:
				for i, video := range query["video"] {
					if video == "copy" {
						s += " -map 0:v:0 -c:v:" + strconv.Itoa(i) + " copy"
					} else {
						s += " -map 0:v:0 " + strings.ReplaceAll(tpl[video], ":v ", ":v:"+strconv.Itoa(i)+" ")
					}
				}
			}

			switch len(query["audio"]) {
			case 0:
				s += " -an"
			case 1:
				if len(query["video"]) > 1 {
					s += " -map 0:a:0"
				}
				for _, audio := range query["audio"] {
					if audio == "copy" {
						s += " -c:a copy"
					} else {
						s += " " + tpl[audio]
					}
				}
			default:
				for i, audio := range query["audio"] {
					if audio == "copy" {
						s += " -map 0:a:0 -c:a:" + strconv.Itoa(i) + " copy"
					} else {
						s += " -map 0:a:0 " + strings.ReplaceAll(tpl[audio], ":a ", ":a:"+strconv.Itoa(i)+" ")
					}
				}
			}
		} else {
			s += " -c copy"
		}

		s += " " + tpl["output"]

		return exec.Handle(s)
	})

	device.Bin = cfg.Mod["bin"]
	device.Init()
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
