package main

import (
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/shell"
)

func main() {
	app.Init()
	streams.Init()

	rtsp.Init()

	shell.RunUntilSignal()
}
