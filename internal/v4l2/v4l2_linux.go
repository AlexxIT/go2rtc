package v4l2

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/v4l2"
	"github.com/AlexxIT/go2rtc/pkg/v4l2/device"
)

func Init() {
	streams.HandleFunc("v4l2", func(source string) (core.Producer, error) {
		return v4l2.Open(source)
	})

	api.HandleFunc("api/v4l2", apiV4L2)
}

func apiV4L2(w http.ResponseWriter, r *http.Request) {
	files, err := os.ReadDir("/dev")
	if err != nil {
		return
	}

	var sources []*api.Source

	for _, file := range files {
		if !strings.HasPrefix(file.Name(), core.KindVideo) {
			continue
		}

		path := "/dev/" + file.Name()

		dev, err := device.Open(path)
		if err != nil {
			continue
		}

		formats, _ := dev.ListFormats()
		for _, fourCC := range formats {
			name, ffmpeg := findFormat(fourCC)
			source := &api.Source{Name: name}

			sizes, _ := dev.ListSizes(fourCC)
			for _, wh := range sizes {
				if source.Info != "" {
					source.Info += " "
				}

				source.Info += fmt.Sprintf("%dx%d", wh[0], wh[1])

				frameRates, _ := dev.ListFrameRates(fourCC, wh[0], wh[1])
				for _, fr := range frameRates {
					source.Info += fmt.Sprintf("@%d", fr)

					if source.URL == "" && ffmpeg != "" {
						source.URL = fmt.Sprintf(
							"v4l2:device?video=%s&input_format=%s&video_size=%dx%d&framerate=%d",
							path, ffmpeg, wh[0], wh[1], fr,
						)
					}
				}
			}

			if source.Info != "" {
				sources = append(sources, source)
			}
		}

		_ = dev.Close()
	}

	api.ResponseSources(w, sources)
}

func findFormat(fourCC uint32) (name, ffmpeg string) {
	for _, format := range device.Formats {
		if format.FourCC == fourCC {
			return format.Name, format.FFmpeg
		}
	}
	return string(binary.LittleEndian.AppendUint32(nil, fourCC)), ""
}
