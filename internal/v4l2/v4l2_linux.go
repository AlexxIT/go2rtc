//go:build linux && (386 || arm || amd64 || arm64)

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
			source := &api.Source{}

			for _, format := range device.Formats {
				if format.FourCC == fourCC {
					source.Name = format.Name
					source.URL = "v4l2:device?video=" + path + "&input_format=" + format.FFmpeg + "&video_size="
					break
				}
			}

			if source.Name != "" {
				sizes, _ := dev.ListSizes(fourCC)
				for i := 0; i < len(sizes); i += 2 {
					size := fmt.Sprintf("%dx%d", sizes[i], sizes[i+1])
					if i > 0 {
						source.Info += " " + size
					} else {
						source.Info = size
						source.URL += size
					}
				}
			} else {
				source.Name = string(binary.LittleEndian.AppendUint32(nil, fourCC))
			}

			sources = append(sources, source)
		}

		_ = dev.Close()
	}

	api.ResponseSources(w, sources)
}
