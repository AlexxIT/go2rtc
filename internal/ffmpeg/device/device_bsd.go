//go:build freebsd || netbsd || openbsd || dragonfly

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
		input := "-f oss"

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

		// [video4linux2,v4l2 @ 0x860b92280] Raw       :     yuyv422 :           YUYV 4:2:2 : 640x480 160x120 176x144 320x176 320x240 352x288 432x240 544x288 640x360 752x416 800x448 800x600 864x480 960x544 960x720 1024x576 1184x656 1280x720 1280x960
		// [video4linux2,v4l2 @ 0x860b92280] Compressed:       mjpeg :          Motion-JPEG : 640x480 160x120 176x144 320x176 320x240 352x288 432x240 544x288 640x360 752x416 800x448 800x600 864x480 960x544 960x720 1024x576 1184x656 1280x720 1280x960
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

	err = exec.Command(Bin, "-f", "oss", "-i", "/dev/dsp", "-t", "1", "-f", "null", "-").Run()
	if err == nil {
		stream := &api.Source{
			Name: "OSS default",
			Info: " ",
			URL:  "ffmpeg:device?audio=default&channels=1&sample_rate=16000&#audio=opus",
		}

		audios = append(audios, "default")
		streams = append(streams, stream)
	}
}
