package main

import (
	"os"
	"runtime"

	"syscall"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
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
	"github.com/AlexxIT/go2rtc/internal/nest"
	"github.com/AlexxIT/go2rtc/internal/ngrok"
	"github.com/AlexxIT/go2rtc/internal/onvif"
	"github.com/AlexxIT/go2rtc/internal/roborock"
	"github.com/AlexxIT/go2rtc/internal/rtmp"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/srtp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/internal/tapo"
	"github.com/AlexxIT/go2rtc/internal/webrtc"
	"github.com/AlexxIT/go2rtc/internal/webtorrent"
	"github.com/AlexxIT/go2rtc/pkg/shell"

	"github.com/rs/zerolog/log"
	daemon "github.com/sevlyar/go-daemon"
)

var (
	stop = make(chan struct{})
	done = make(chan struct{})
)

func main() {
	shell.Init()

	app.Init() // init config and logs

	api.Init() // init API before all others

	if shell.Daemonize {
		cntxt := &daemon.Context{
			PidFileName: shell.PidFilePath,
			PidFilePerm: 0644,
			LogFileName: app.GetLogFilepath(),
			LogFilePerm: 0644,
		}

		daemon.SetSigHandler(termHandler, syscall.SIGQUIT)
		daemon.SetSigHandler(termHandler, syscall.SIGTERM)
		daemon.SetSigHandler(termHandler, syscall.SIGSEGV)
		daemon.SetSigHandler(reloadHandler, syscall.SIGHUP)

		d, err := cntxt.Reborn()
		if err != nil {
			log.Fatal().Err(err)
		}
		if d != nil {
			log.Info().Msgf("daemon started with pid %d", d.Pid)
			return
		}
		defer cntxt.Release()

		//log.Debug().Msg("- - - - - - - - - - - - - - -")

		go looper()

		err = daemon.ServeSignals()
		if err != nil {
			log.Printf("Error: %s", err.Error())
		}

		log.Info().Msg("daemon terminated")
	} else {
		mainLoop()
	}
}

func looper() {
LOOP:
	for {
		mainLoop()
		select {
		case <-stop:
			break LOOP
		default:
		}
	}
	done <- struct{}{}
}

func mainLoop() {
	if runtime.GOOS != "windows" && (os.Getuid() != int(shell.GetForkUserId())) { // user sets by CLI
		shell.CheckRootAndDropPrivileges()
	}

	// 1. Core modules: app, api/ws, streams

	ws.Init() // init WS API endpoint

	streams.Init() // streams module

	// 2. Main sources and servers

	rtsp.Init()   // rtsp source, RTSP server
	webrtc.Init() // webrtc source, WebRTC server

	// 3. Main API

	mp4.Init()   // MP4 API
	hls.Init()   // HLS API
	mjpeg.Init() // MJPEG API

	// 4. Other sources and servers

	hass.Init()       // hass source, Hass API server
	onvif.Init()      // onvif source, ONVIF API server
	webtorrent.Init() // webtorrent source, WebTorrent module

	// 5. Other sources

	rtmp.Init()     // rtmp source
	exec.Init()     // exec source
	ffmpeg.Init()   // ffmpeg source
	echo.Init()     // echo source
	ivideon.Init()  // ivideon source
	http.Init()     // http/tcp source
	dvrip.Init()    // dvrip source
	tapo.Init()     // tapo source
	isapi.Init()    // isapi source
	mpegts.Init()   // mpegts passive source
	roborock.Init() // roborock source
	homekit.Init()  // homekit source
	nest.Init()     // nest source

	// 6. Helper modules

	ngrok.Init() // Ngrok module
	srtp.Init()  // SRTP server
	debug.Init() // debug API

	// 7. Go

	shell.RunUntilSignal()
}
func termHandler(sig os.Signal) error {
	log.Debug().Msg("terminating...")
	stop <- struct{}{}
	if sig == syscall.SIGQUIT {
		<-done
	}
	return daemon.ErrStop
}

func reloadHandler(sig os.Signal) error {
	log.Info().Msg("Not implemented yet :)")
	return nil
}
