package exec

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/rtsp"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	pkg "github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/rs/zerolog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

func Init() {
	// depends on RTSP server
	if rtsp.Port == "" {
		return
	}

	rtsp.HandleFunc(func(conn *pkg.Conn) bool {
		waitersMu.Lock()
		waiter := waiters[conn.URL.Path]
		waitersMu.Unlock()

		if waiter == nil {
			return false
		}

		// unblocking write to channel
		select {
		case waiter <- conn:
			return true
		default:
			return false
		}
	})

	streams.HandleFunc("exec", Handle)

	log = app.GetLogger("exec")
}

func Handle(url string) (core.Producer, error) {
	sum := md5.Sum([]byte(url))
	path := "/" + hex.EncodeToString(sum[:])

	url = strings.Replace(
		url, "{output}", "rtsp://127.0.0.1:"+rtsp.Port+path, 1,
	)

	// remove `exec:`
	args := shell.QuoteSplit(url[5:])
	cmd := exec.Command(args[0], args[1:]...)

	if log.Trace().Enabled() {
		cmd.Stdout = os.Stdout
	}
	if log.Debug().Enabled() {
		cmd.Stderr = os.Stderr
	}

	ch := make(chan core.Producer)

	waitersMu.Lock()
	waiters[path] = ch
	waitersMu.Unlock()

	defer func() {
		waitersMu.Lock()
		delete(waiters, path)
		waitersMu.Unlock()
	}()

	log.Debug().Str("url", url).Msg("[exec] run")

	ts := time.Now()

	if err := cmd.Start(); err != nil {
		log.Error().Err(err).Str("url", url).Msg("[exec]")
		return nil, err
	}

	chErr := make(chan error)

	go func() {
		err := cmd.Wait()
		// unblocking write to channel
		select {
		case chErr <- err:
		default:
			log.Trace().Str("url", url).Msg("[exec] close")
		}
	}()

	select {
	case <-time.After(time.Second * 60):
		_ = cmd.Process.Kill()
		log.Error().Str("url", url).Msg("[exec] timeout")
		return nil, errors.New("timeout")
	case err := <-chErr:
		return nil, fmt.Errorf("exec: %s", err)
	case prod := <-ch:
		log.Debug().Stringer("launch", time.Since(ts)).Msg("[exec] run")
		return prod, nil
	}
}

// internal

var log zerolog.Logger
var waiters = map[string]chan core.Producer{}
var waitersMu sync.Mutex
