package ffmpeg

import (
	"bytes"
	"fmt"
	"net/url"
	"os/exec"

	"github.com/AlexxIT/go2rtc/internal/ffmpeg/hardware"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/ffmpeg"
	"github.com/AlexxIT/go2rtc/pkg/shell"
)

func JPEGWithQuery(b []byte, query url.Values) ([]byte, error) {
	args := parseQuery(query)
	return transcode(b, args.String())
}

func JPEGWithScale(b []byte, width, height int) ([]byte, error) {
	args := defaultArgs()
	args.AddFilter(fmt.Sprintf("scale=%d:%d", width, height))
	return transcode(b, args.String())
}

func transcode(b []byte, args string) ([]byte, error) {
	cmdArgs := shell.QuoteSplit(args)
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = bytes.NewBuffer(b)
	return cmd.Output()
}

func defaultArgs() *ffmpeg.Args {
	return &ffmpeg.Args{
		Bin:    defaults["bin"],
		Global: defaults["global"],
		Input:  "-i -",
		Codecs: []string{defaults["mjpeg"]},
		Output: defaults["output/mjpeg"],
	}
}

func parseQuery(query url.Values) *ffmpeg.Args {
	args := defaultArgs()

	var width = -1
	var height = -1
	var r, hw string

	for k, v := range query {
		switch k {
		case "width", "w":
			width = core.Atoi(v[0])
		case "height", "h":
			height = core.Atoi(v[0])
		case "rotate":
			r = v[0]
		case "hardware", "hw":
			hw = v[0]
		}
	}

	if width > 0 || height > 0 {
		args.AddFilter(fmt.Sprintf("scale=%d:%d", width, height))
	}

	if r != "" {
		switch r {
		case "90":
			args.AddFilter("transpose=1") // 90 degrees clockwise
		case "180":
			args.AddFilter("transpose=1,transpose=1")
		case "-90", "270":
			args.AddFilter("transpose=2") // 90 degrees counterclockwise
		}
	}

	if hw != "" {
		hardware.MakeHardware(args, hw, defaults)
	}

	return args
}
