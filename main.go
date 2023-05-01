package main

import (
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/debug"
	"github.com/AlexxIT/go2rtc/internal/dvrip"
	"github.com/AlexxIT/go2rtc/internal/echo"
	"github.com/AlexxIT/go2rtc/internal/exec"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg"
	"github.com/AlexxIT/go2rtc/internal/hass"
	"github.com/AlexxIT/go2rtc/internal/hls"
	"github.com/AlexxIT/go2rtc/internal/homekit"
	"github.com/AlexxIT/go2rtc/internal/http"
	"github.com/AlexxIT/go2rtc/internal/isapi"
	"github.com/AlexxIT/go2rtc/internal/ivideon"
	"github.com/AlexxIT/go2rtc/internal/mjpeg"
	"github.com/AlexxIT/go2rtc/internal/mp4"
	"github.com/AlexxIT/go2rtc/internal/mpegts"
	"github.com/AlexxIT/go2rtc/internal/ngrok"
	"github.com/AlexxIT/go2rtc/internal/onvif"
	"github.com/AlexxIT/go2rtc/internal/roborock"
	"github.com/AlexxIT/go2rtc/internal/rtmp"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/srtp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/internal/tapo"
	"github.com/AlexxIT/go2rtc/internal/tcp"
	"github.com/AlexxIT/go2rtc/internal/webrtc"
	"github.com/AlexxIT/go2rtc/internal/webtorrent"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	app.Init()     // init config and logs
	api.Init()     // init HTTP API server
	streams.Init() // load streams list
	onvif.Init()

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
	roborock.Init()
	tcp.Init()

	srtp.Init()
	homekit.Init()

	webrtc.Init()
	mp4.Init()
	hls.Init()
	mjpeg.Init()

	webtorrent.Init()
	ngrok.Init()
	debug.Init()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	println("exit OK")
}
