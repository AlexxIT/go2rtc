package ffmpeg

import (
	"bytes"
	"strconv"
	"strings"
)

// correlation of libavformat versions with ffmpeg versions
const (
	Version50 = "59. 16"
	Version51 = "59. 27"
	Version60 = "60.  3"
	Version61 = "60. 16"
	Version70 = "61.  1"
)

type Args struct {
	Bin     string   // ffmpeg
	Global  string   // -hide_banner -v error
	Input   string   // -re -stream_loop -1 -i /media/bunny.mp4
	Codecs  []string // -c:v libx264 -g:v 30 -preset:v ultrafast -tune:v zerolatency
	Filters []string // scale=1920:1080
	Output  string   // -f rtsp {output}
	Version string   // libavformat version, it's more reliable than the ffmpeg version

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
	// starting from FFmpeg 6.1 readrate=1 has default initial bust 0.5 sec
	// it might make us miss the first couple seconds of the file
	if strings.HasPrefix(a.Input, "-re ") && a.Version >= Version61 {
		b.WriteString("-readrate_initial_burst 0.001 ")
	}
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
	}

	b.WriteByte(' ')
	b.WriteString(a.Output)

	return b.String()
}

func ParseVersion(b []byte) (ffmpeg string, libavformat string) {
	if len(b) > 100 {
		// ffmpeg version n7.0-30-g8b0fe91754-20240520 Copyright (c) 2000-2024 the FFmpeg developers
		if i := bytes.IndexByte(b[15:], ' '); i > 0 {
			ffmpeg = string(b[15 : 15+i])
		}

		// libavformat    60. 16.100 / 60. 16.100
		if i := strings.Index(string(b), "libavformat"); i > 0 {
			libavformat = string(b[i+15 : i+25])
		}
	}
	return
}
