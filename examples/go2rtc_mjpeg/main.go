package main

import (
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/mjpeg"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/internal/v4l2"
	"github.com/AlexxIT/go2rtc/pkg/shell"
)

func main() {
	app.Init()
	streams.Init()

	api.Init()
	mjpeg.Init()
	v4l2.Init()

	shell.RunUntilSignal()
}
