//go:build unix && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package device

import (
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func queryToInput(query url.Values) string {
	if video := query.Get("video"); video != "" {
		// https://ffmpeg.org/ffmpeg-devices.html#video4linux2_002c-v4l2
		input := "-f v4l2"

		for key, value := range query {
			switch key {
			case "resolution":
				input += " -video_size " + value[0]
			case "video_size", "pixel_format", "input_format", "framerate", "use_libv4l2":
				input += " -" + key + " " + value[0]
			}
		}

		return input + " -i " + indexToItem(videos, video)
	}

	if audio := query.Get("audio"); audio != "" {
		// https://trac.ffmpeg.org/wiki/Capture/ALSA
		input := "-f alsa"

		for key, value := range query {
			switch key {
			case "channels", "sample_rate":
				input += " -" + key + " " + value[0]
			}
		}

		return input + " -i " + indexToItem(audios, audio)
	}

	return ""
}

func initDevices() {
	files, err := os.ReadDir("/dev")
	if err != nil {
		return
	}

	for _, file := range files {
		if !strings.HasPrefix(file.Name(), core.KindVideo) {
			continue
		}

		name := "/dev/" + file.Name()

		cmd := exec.Command(
			Bin, "-hide_banner", "-f", "v4l2", "-list_formats", "all", "-i", name,
		)
		b, _ := cmd.CombinedOutput()

		// [video4linux2,v4l2 @ 0x204e1c0] Compressed:       mjpeg :          Motion-JPEG : 640x360 1280x720 1920x1080
		// [video4linux2,v4l2 @ 0x204e1c0] Raw       :     yuyv422 :           YUYV 4:2:2 : 640x360 1280x720 1920x1080
		// [video4linux2,v4l2 @ 0x204e1c0] Compressed:        h264 :                H.264 : 640x360 1280x720 1920x1080
		re := regexp.MustCompile("(Raw *|Compressed): +(.+?) : +(.+?) : (.+)")
		m := re.FindAllStringSubmatch(string(b), -1)
		for _, i := range m {
			size, _, _ := strings.Cut(i[4], " ")
			stream := &api.Source{
				Name: i[3],
				Info: i[4],
				URL:  "ffmpeg:device?video=" + name + "&input_format=" + i[2] + "&video_size=" + size,
			}

			if i[1] != "Compressed" {
				stream.URL += "#video=h264#hardware"
			}

			videos = append(videos, name)
			streams = append(streams, stream)
		}
	}

	err = exec.Command(Bin, "-f", "alsa", "-i", "default", "-t", "1", "-f", "null", "-").Run()
	if err == nil {
		stream := &api.Source{
			Name: "ALSA default",
			Info: " ",
			URL:  "ffmpeg:device?audio=default&channels=1&sample_rate=16000&#audio=opus",
		}

		audios = append(audios, "default")
		streams = append(streams, stream)
	}
}
