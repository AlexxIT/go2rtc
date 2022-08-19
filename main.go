package main

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/exec"
	"github.com/AlexxIT/go2rtc/cmd/ffmpeg"
	"github.com/AlexxIT/go2rtc/cmd/hass"
	"github.com/AlexxIT/go2rtc/cmd/mse"
	"github.com/AlexxIT/go2rtc/cmd/ngrok"
	"github.com/AlexxIT/go2rtc/cmd/rtmp"
	"github.com/AlexxIT/go2rtc/cmd/rtsp"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/cmd/webrtc"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	app.Init()     // init config and logs
	streams.Init() // load streams list

	rtsp.Init()   // add support RTSP client and RTSP server
	rtmp.Init()   // add support RTMP client
	exec.Init()   // add support exec scheme (depends on RTSP server)
	ffmpeg.Init() // add support ffmpeg scheme (depends on exec scheme)
	hass.Init()   // add support hass scheme

	api.Init() // init HTTP API server

	webrtc.Init()
	mse.Init()

	ngrok.Init()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	println("exit OK")
}
