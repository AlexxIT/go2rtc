package ffmpeg

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Args struct {
	Bin     string   // ffmpeg
	Global  string   // -hide_banner -v error
	Input   string   // -re -stream_loop -1 -i /media/bunny.mp4
	Codecs  []string // -c:v libx264 -g:v 30 -preset:v ultrafast -tune:v zerolatency
	Filters []string // scale=1920:1080
	Output  string   // -f rtsp {output}

	Video, Audio int // count of Video and Audio params
}

func (a *Args) AddCodec(codec string) {
	a.Codecs = append(a.Codecs, codec)
}

func (a *Args) AddFilter(filter string) {
	a.Filters = append(a.Filters, filter)
}

func (a *Args) InsertFilter(filter string) {
	a.Filters = append([]string{filter}, a.Filters...)
}

func (a *Args) HasFilters(filters ...string) bool {
	for _, f1 := range a.Filters {
		for _, f2 := range filters {
			if strings.HasPrefix(f1, f2) {
				return true
			}
		}
	}

	return false
}

func (a *Args) String() string {
	b := bytes.NewBuffer(make([]byte, 0, 512))

	b.WriteString(a.Bin)

	if a.Global != "" {
		b.WriteByte(' ')
		b.WriteString(a.Global)
	}

	b.WriteByte(' ')
	b.WriteString(a.Input)

	multimode := a.Video > 1 || a.Audio > 1
	var iv, ia int

	for _, codec := range a.Codecs {
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

	if len(a.Filters) > 0 {
		for i, filter := range a.Filters {
			if i == 0 {
				b.WriteString(` -vf "`)
			} else {
				b.WriteByte(',')
			}
			b.WriteString(filter)
		}
		b.WriteByte('"')
		ffmpegVersion, _ := a.GetFFmpegVersion()
		if core.CompareVersions(ffmpegVersion, "7.0") >= 0 {

			b.WriteString(fmt.Sprintf(` -filter_complex_threads %d`, core.MaxCPUThreads(1)))
		}
	}

	b.WriteByte(' ')
	b.WriteString(a.Output)

	return b.String()
}

func (a *Args) GetFFmpegVersion() (string, error) {
	cmd := exec.Command(a.Bin, "-version")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	firstLine := strings.Split(out.String(), "\n")[0]
	versionInfo := strings.Fields(firstLine)[2]
	return versionInfo, nil
}
