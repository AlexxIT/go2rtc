package main

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/debug"
	"github.com/AlexxIT/go2rtc/cmd/dvrip"
	"github.com/AlexxIT/go2rtc/cmd/echo"
	"github.com/AlexxIT/go2rtc/cmd/exec"
	"github.com/AlexxIT/go2rtc/cmd/ffmpeg"
	"github.com/AlexxIT/go2rtc/cmd/hass"
	"github.com/AlexxIT/go2rtc/cmd/hls"
	"github.com/AlexxIT/go2rtc/cmd/homekit"
	"github.com/AlexxIT/go2rtc/cmd/http"
	"github.com/AlexxIT/go2rtc/cmd/isapi"
	"github.com/AlexxIT/go2rtc/cmd/ivideon"
	"github.com/AlexxIT/go2rtc/cmd/mjpeg"
	"github.com/AlexxIT/go2rtc/cmd/mp4"
	"github.com/AlexxIT/go2rtc/cmd/mpegts"
	"github.com/AlexxIT/go2rtc/cmd/ngrok"
	"github.com/AlexxIT/go2rtc/cmd/rtmp"
	"github.com/AlexxIT/go2rtc/cmd/rtsp"
	"github.com/AlexxIT/go2rtc/cmd/srtp"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/cmd/tapo"
	"github.com/AlexxIT/go2rtc/cmd/webrtc"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	app.Init()     // init config and logs
	api.Init()     // init HTTP API server
	streams.Init() // load streams list

	rtsp.Init()   // add support RTSP client and RTSP server
	rtmp.Init()   // add support RTMP client
	exec.Init()   // add support exec scheme (depends on RTSP server)
	ffmpeg.Init() // add support ffmpeg scheme (depends on exec scheme)
	hass.Init()   // add support hass scheme
	echo.Init()
	ivideon.Init()
	http.Init()
	dvrip.Init()
	tapo.Init()
	isapi.Init()
	mpegts.Init()

	srtp.Init()
	homekit.Init()

	webrtc.Init()
	mp4.Init()
	hls.Init()
	mjpeg.Init()

	ngrok.Init()
	debug.Init()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	println("exit OK")
}
