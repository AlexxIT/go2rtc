package device

import (
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// https://trac.ffmpeg.org/wiki/Capture/Webcam
const deviceInputPrefix = "-f v4l2"

func deviceInputSuffix(video, audio string) string {
	if video != "" {
		return video
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
			stream := api.Stream{
				Name: i[3] + " | " + i[4],
				URL:  "ffmpeg:device?video=" + name + "&input_format=" + i[2] + "&video_size=" + size,
			}

			if i[1] != "Compressed" {
				stream.URL += "#video=h264#hardware"
			}

			videos = append(videos, name)
			streams = append(streams, stream)
		}
	}
}
