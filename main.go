package main

import (
	"slices"

	"github.com/VaBanck/go2rtc/internal/alsa"
	"github.com/VaBanck/go2rtc/internal/api"
	"github.com/VaBanck/go2rtc/internal/api/ws"
	"github.com/VaBanck/go2rtc/internal/app"
	"github.com/VaBanck/go2rtc/internal/bubble"
	"github.com/VaBanck/go2rtc/internal/debug"
	"github.com/VaBanck/go2rtc/internal/doorbird"
	"github.com/VaBanck/go2rtc/internal/dvrip"
	"github.com/VaBanck/go2rtc/internal/echo"
	"github.com/VaBanck/go2rtc/internal/eseecloud"
	"github.com/VaBanck/go2rtc/internal/exec"
	"github.com/VaBanck/go2rtc/internal/expr"
	"github.com/VaBanck/go2rtc/internal/ffmpeg"
	"github.com/VaBanck/go2rtc/internal/flussonic"
	"github.com/VaBanck/go2rtc/internal/gopro"
	"github.com/VaBanck/go2rtc/internal/hass"
	"github.com/VaBanck/go2rtc/internal/hls"
	"github.com/VaBanck/go2rtc/internal/homekit"
	"github.com/VaBanck/go2rtc/internal/http"
	"github.com/VaBanck/go2rtc/internal/isapi"
	"github.com/VaBanck/go2rtc/internal/ivideon"
	"github.com/VaBanck/go2rtc/internal/kasa"
	"github.com/VaBanck/go2rtc/internal/mjpeg"
	"github.com/VaBanck/go2rtc/internal/mp4"
	"github.com/VaBanck/go2rtc/internal/webp"
	"github.com/VaBanck/go2rtc/internal/mpeg"
	"github.com/VaBanck/go2rtc/internal/multitrans"
	"github.com/VaBanck/go2rtc/internal/nest"
	"github.com/VaBanck/go2rtc/internal/ngrok"
	"github.com/VaBanck/go2rtc/internal/onvif"
	"github.com/VaBanck/go2rtc/internal/pinggy"
	"github.com/VaBanck/go2rtc/internal/ring"
	"github.com/VaBanck/go2rtc/internal/roborock"
	"github.com/VaBanck/go2rtc/internal/rtmp"
	"github.com/VaBanck/go2rtc/internal/rtsp"
	"github.com/VaBanck/go2rtc/internal/srtp"
	"github.com/VaBanck/go2rtc/internal/streams"
	"github.com/VaBanck/go2rtc/internal/tapo"
	"github.com/VaBanck/go2rtc/internal/tuya"
	"github.com/VaBanck/go2rtc/internal/v4l2"
	"github.com/VaBanck/go2rtc/internal/webcodecs"
	"github.com/VaBanck/go2rtc/internal/webrtc"
	"github.com/VaBanck/go2rtc/internal/webtorrent"
	"github.com/VaBanck/go2rtc/internal/wyoming"
	"github.com/VaBanck/go2rtc/internal/wyze"
	"github.com/VaBanck/go2rtc/internal/xiaomi"
	"github.com/VaBanck/go2rtc/internal/yandex"
	"github.com/VaBanck/go2rtc/pkg/shell"
)

func main() {
	// version will be set later from -buildvcs info, this used only as fallback
	app.Version = "1.9.17"

	type module struct {
		name string
		init func()
	}

	modules := []module{
		{"", app.Init},    // init config and logs
		{"api", api.Init}, // init API before all others
		{"ws", ws.Init},   // init WS API endpoint
		{"", streams.Init},
		// Main sources and servers
		{"http", http.Init},     // rtsp source, HTTP server
		{"rtsp", rtsp.Init},     // rtsp source, RTSP server
		{"webrtc", webrtc.Init}, // webrtc source, WebRTC server
		// Main API
		{"mp4", mp4.Init},     // MP4 API
		{"webcodecs", webcodecs.Init}, // WebCodecs API
		{"hls", hls.Init},     // HLS API
		{"mjpeg", mjpeg.Init}, // MJPEG API
		{"webp", webp.Init},   // WebP API
		// Other sources and servers
		{"hass", hass.Init},             // hass source, Hass API server
		{"homekit", homekit.Init},       // homekit source, HomeKit server
		{"onvif", onvif.Init},           // onvif source, ONVIF API server
		{"rtmp", rtmp.Init},             // rtmp source, RTMP server
		{"webtorrent", webtorrent.Init}, // webtorrent source, WebTorrent module
		{"wyoming", wyoming.Init},
		// Exec and script sources
		{"echo", echo.Init},
		{"exec", exec.Init},
		{"expr", expr.Init},
		{"ffmpeg", ffmpeg.Init},
		// Hardware sources
		{"alsa", alsa.Init},
		{"v4l2", v4l2.Init},
		// Other sources
		{"bubble", bubble.Init},
		{"doorbird", doorbird.Init},
		{"dvrip", dvrip.Init},
		{"eseecloud", eseecloud.Init},
		{"flussonic", flussonic.Init},
		{"gopro", gopro.Init},
		{"isapi", isapi.Init},
		{"ivideon", ivideon.Init},
		{"kasa", kasa.Init},
		{"mpegts", mpeg.Init},
		{"multitrans", multitrans.Init},
		{"nest", nest.Init},
		{"ring", ring.Init},
		{"roborock", roborock.Init},
		{"tapo", tapo.Init},
		{"tuya", tuya.Init},
		{"wyze", wyze.Init},
		{"xiaomi", xiaomi.Init},
		{"yandex", yandex.Init},
		// Helper modules
		{"debug", debug.Init},
		{"ngrok", ngrok.Init},
		{"pinggy", pinggy.Init},
		{"srtp", srtp.Init},
	}

	for _, m := range modules {
		if app.Modules == nil || m.name == "" || slices.Contains(app.Modules, m.name) {
			m.init()
		}
	}

	streams.SetReady()

	shell.RunUntilSignal()
}
