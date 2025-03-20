//go:build linux && (386 || amd64 || arm || arm64 || mipsle)

package alsa

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/alsa"
	"github.com/AlexxIT/go2rtc/pkg/alsa/device"
)

func Init() {
	streams.HandleFunc("alsa", alsa.Open)

	api.HandleFunc("api/alsa", apiAlsa)
}

func apiAlsa(w http.ResponseWriter, r *http.Request) {
	files, err := os.ReadDir("/dev/snd/")
	if err != nil {
		return
	}

	var sources []*api.Source

	for _, file := range files {
		if !strings.HasPrefix(file.Name(), "pcm") {
			continue
		}

		path := "/dev/snd/" + file.Name()

		dev, err := device.Open(path)
		if err != nil {
			continue
		}

		info, err := dev.Info()
		if err == nil {
			formats := formatsToString(dev.ListFormats())
			r1, r2 := dev.RangeSampleRates()
			c1, c2 := dev.RangeChannels()
			source := &api.Source{
				Name: info.ID + " / " + info.Name + " / " + info.SubName,
				Info: fmt.Sprintf("Formats: %s, Rates: %d-%d, Channels: %d-%d", formats, r1, r2, c1, c2),
				URL:  "alsa:device?audio=" + path,
			}
			sources = append(sources, source)
		}

		_ = dev.Close()
	}

	api.ResponseSources(w, sources)
}

func formatsToString(formats []byte) string {
	var s string
	for i, format := range formats {
		if i > 0 {
			s += " "
		}
		switch format {
		case 2:
			s += "s16le"
		case 10:
			s += "s32le"
		default:
			s += strconv.Itoa(int(format))
		}

	}
	return s
}
