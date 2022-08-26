package ffmpeg

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/exec"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"net/url"
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
		"link": "-i {input}",
		"rtsp": "-fflags nobuffer -flags low_delay -rtsp_transport tcp -i {input}",
		"file": "-re -stream_loop -1 -i {input}",

		// output
		"out": "-rtsp_transport tcp -f rtsp {output}",

		// `-g 30` - group of picture, GOP, keyframe interval
		// `-preset superfast` - we can't use ultrafast because it doesn't support `-profile main -level 4.1`
		// `-tune zerolatency` - for minimal latency
		// `-profile main -level 4.1` - most used streaming profile
		// `-pix_fmt yuv420p` - if input pix format 4:2:2
		"h264":       "-codec:v libx264 -g 30 -preset superfast -tune zerolatency -profile main -level 4.1 -pix_fmt yuv420p",
		"h264/ultra": "-codec:v libx264 -g 30 -preset ultrafast -tune zerolatency",
		"h264/high":  "-codec:v libx264 -g 30 -preset superfast -tune zerolatency",
		"h265":       "-codec:v libx265 -g 30 -preset ultrafast -tune zerolatency",
		"opus":       "-codec:a libopus -ar 48000 -ac 2",
		"pcmu":       "-codec:a pcm_mulaw -ar 8000 -ac 1",
		"pcmu/16000": "-codec:a pcm_mulaw -ar 16000 -ac 1",
		"pcmu/48000": "-codec:a pcm_mulaw -ar 48000 -ac 1",
		"pcma":       "-codec:a pcm_alaw -ar 8000 -ac 1",
		"pcma/16000": "-codec:a pcm_alaw -ar 16000 -ac 1",
		"pcma/48000": "-codec:a pcm_alaw -ar 48000 -ac 1",
		"aac/16000":  "-codec:a aac -ar 16000 -ac 1",
	}

	app.LoadConfig(&cfg)

	tpl = cfg.Mod

	streams.HandleFunc("ffmpeg", func(s string) (streamer.Producer, error) {
		s = s[7:] // remove `ffmpeg:`

		var query url.Values
		if i := strings.IndexByte(s, '#'); i > 0 {
			query = parseQuery(s[i+1:])
			s = s[:i]
		}

		var template string
		switch {
		case strings.HasPrefix(s, "rtsp"):
			template = tpl["rtsp"]
		case strings.HasPrefix(s, "device"):
			template, _ = getDevice(s)
		case strings.Contains(s, "://"):
			template = tpl["link"]
		default:
			template = tpl["file"]
		}

		s = "exec:" + tpl["bin"] + " -hide_banner " +
			strings.Replace(template, "{input}", s, 1)

		if query != nil {
			for _, raw := range query["raw"] {
				s += " " + raw
			}

			// TODO: multiple codecs via -map
			// s += fmt.Sprintf(" -map 0:v:0 -c:v:%d copy", i)

			for _, video := range query["video"] {
				if video == "copy" {
					s += " -codec:v copy"
				} else {
					s += " " + tpl[video]
				}
			}

			for _, audio := range query["audio"] {
				if audio == "copy" {
					s += " -codec:v copy"
				} else {
					s += " " + tpl[audio]
				}
			}

			if query["video"] == nil {
				s += " -vn"
			}
			if query["audio"] == nil {
				s += " -an"
			}
		} else {
			s += " -c copy"
		}

		s += " " + tpl["out"]

		return exec.Handle(s)
	})

	api.HandleFunc("/api/devices", handleDevices)
}

var tpl map[string]string

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
