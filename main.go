package main

import (
	"os"

	"syscall"

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
	//pidfile := app.GetPidFilePath()
	if shell.Daemonize {
		cntxt := &daemon.Context{
			PidFileName: shell.PidFilePath,
			PidFilePerm: 0644,
			LogFileName: app.GetLogFilepath(),
			LogFilePerm: 0644,
			//WorkDir: "./",
			//Umask:   027,
			//Args:        []string{"[go-daemon sample]"},
			Credential: &daemon.Credential{
				Uid:         shell.GetForkUserId(),
				Gid:         shell.GetForkGroupId(),
				Groups:      nil,
				NoSetGroups: true,
			},
		}
		if len(daemon.ActiveFlags()) > 0 {
			d, err := cntxt.Search()
			if err != nil {
				log.Fatal().Err(err).Msgf("Unable send signal to the daemon: %s", err.Error())
			}
			daemon.SendCommands(d)
			return
		}

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
	nest.Init()

	srtp.Init()
	homekit.Init()

	webrtc.Init()
	mp4.Init()
	hls.Init()
	mjpeg.Init()

	webtorrent.Init()
	ngrok.Init()
	debug.Init()

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
