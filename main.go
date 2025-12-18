package main

import (
	"slices"

	"github.com/AlexxIT/go2rtc/internal/alsa"
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/bubble"
	"github.com/AlexxIT/go2rtc/internal/debug"
	"github.com/AlexxIT/go2rtc/internal/doorbird"
	"github.com/AlexxIT/go2rtc/internal/dvrip"
	"github.com/AlexxIT/go2rtc/internal/echo"
	"github.com/AlexxIT/go2rtc/internal/eseecloud"
	"github.com/AlexxIT/go2rtc/internal/exec"
	"github.com/AlexxIT/go2rtc/internal/expr"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg"
	"github.com/AlexxIT/go2rtc/internal/flussonic"
	"github.com/AlexxIT/go2rtc/internal/gopro"
	"github.com/AlexxIT/go2rtc/internal/hass"
	"github.com/AlexxIT/go2rtc/internal/hls"
	"github.com/AlexxIT/go2rtc/internal/homekit"
	"github.com/AlexxIT/go2rtc/internal/http"
	"github.com/AlexxIT/go2rtc/internal/isapi"
	"github.com/AlexxIT/go2rtc/internal/ivideon"
	"github.com/AlexxIT/go2rtc/internal/mjpeg"
	"github.com/AlexxIT/go2rtc/internal/mp4"
	"github.com/AlexxIT/go2rtc/internal/mpegts"
	"github.com/AlexxIT/go2rtc/internal/nest"
	"github.com/AlexxIT/go2rtc/internal/ngrok"
	"github.com/AlexxIT/go2rtc/internal/onvif"
	"github.com/AlexxIT/go2rtc/internal/pinggy"
	"github.com/AlexxIT/go2rtc/internal/ring"
	"github.com/AlexxIT/go2rtc/internal/roborock"
	"github.com/AlexxIT/go2rtc/internal/rtmp"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/srtp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/internal/tapo"
	"github.com/AlexxIT/go2rtc/internal/tuya"
	"github.com/AlexxIT/go2rtc/internal/v4l2"
	"github.com/AlexxIT/go2rtc/internal/webrtc"
	"github.com/AlexxIT/go2rtc/internal/webtorrent"
	"github.com/AlexxIT/go2rtc/internal/wyoming"
	"github.com/AlexxIT/go2rtc/internal/xiaomi"
	"github.com/AlexxIT/go2rtc/internal/yandex"
	"github.com/AlexxIT/go2rtc/pkg/shell"
)

func main() {
	// version will be set later from -buildvcs info, this used only as fallback
	app.Version = "1.9.13"

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
		{"hls", hls.Init},     // HLS API
		{"mjpeg", mjpeg.Init}, // MJPEG API
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
		{"mpegts", mpegts.Init},
		{"nest", nest.Init},
		{"ring", ring.Init},
		{"roborock", roborock.Init},
		{"tapo", tapo.Init},
		{"tuya", tuya.Init},
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

	shell.RunUntilSignal()
}
