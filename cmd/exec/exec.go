package exec

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/rtsp"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	pkg "github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/rs/zerolog"
	"os"
	"os/exec"
	"strings"
	"time"
)

func Init() {
	// depends on RTSP server
	if rtsp.Port == "" {
		return
	}

	rtsp.OnProducer = func(prod streamer.Producer) bool {
		if conn := prod.(*pkg.Conn); conn != nil {
			if waiter := waiters[conn.URL.Path]; waiter != nil {
				waiter <- prod
				return true
			}
		}
		return false
	}

	streams.HandleFunc("exec", Handle)

	log = app.GetLogger("exec")

	// TODO: add sync.Mutex
	waiters = map[string]chan streamer.Producer{}
}

func Handle(url string) (streamer.Producer, error) {
	sum := md5.Sum([]byte(url))
	path := "/" + hex.EncodeToString(sum[:])

	url = strings.Replace(
		url, "{output}", "rtsp://localhost:"+rtsp.Port+path, 1,
	)

	// remove `exec:`
	args := shell.QuoteSplit(url[5:])
	cmd := exec.Command(args[0], args[1:]...)

	if log.Trace().Enabled() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	ch := make(chan streamer.Producer)

	waiters[path] = ch
	defer delete(waiters, path)

	log.Debug().Str("url", url).Msg("[exec] run")

	ts := time.Now()

	if err := cmd.Start(); err != nil {
		log.Error().Err(err).Str("url", url).Msg("[exec]")
		return nil, err
	}

	select {
	case <-time.After(time.Second * 15):
		_ = cmd.Process.Kill()
		log.Error().Str("url", url).Msg("[exec] timeout")
		return nil, errors.New("timeout")
	case prod := <-ch:
		log.Debug().Stringer("launch", time.Since(ts)).Msg("[exec] run")
		return prod, nil
	}
}

// internal

var log zerolog.Logger
var waiters map[string]chan streamer.Producer
