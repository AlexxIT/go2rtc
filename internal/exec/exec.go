package exec

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	pkg "github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/AlexxIT/go2rtc/pkg/stdin"
	"github.com/rs/zerolog"
)

func Init() {
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

	streams.HandleFunc("exec", execHandle)

	log = app.GetLogger("exec")
}

func execHandle(rawURL string) (core.Producer, error) {
	var path string
	var query url.Values

	// RTSP flow should have `{output}` inside URL
	// pipe flow may have `#{params}` inside URL
	if i := strings.Index(rawURL, "{output}"); i > 0 {
		if rtsp.Port == "" {
			return nil, errors.New("exec: rtsp module disabled")
		}

		sum := md5.Sum([]byte(rawURL))
		path = "/" + hex.EncodeToString(sum[:])
		rawURL = rawURL[:i] + "rtsp://127.0.0.1:" + rtsp.Port + path + rawURL[i+8:]
	} else if i = strings.IndexByte(rawURL, '#'); i > 0 {
		query = streams.ParseQuery(rawURL[i+1:])
		rawURL = rawURL[:i]
	}

	args := shell.QuoteSplit(rawURL[5:]) // remove `exec:`
	cmd := exec.Command(args[0], args[1:]...)
	if log.Debug().Enabled() {
		cmd.Stderr = os.Stderr
	}

	if path == "" {
		return handlePipe(rawURL, cmd, query)
	}

	return handleRTSP(rawURL, cmd, path)
}

func handlePipe(_ string, cmd *exec.Cmd, query url.Values) (core.Producer, error) {
	if query.Get("backchannel") == "1" {
		return stdin.NewClient(cmd)
	}

	r, err := PipeCloser(cmd, query)
	if err != nil {
		return nil, err
	}

	if err = cmd.Start(); err != nil {
		return nil, err
	}

	prod, err := magic.Open(r)
	if err != nil {
		_ = r.Close()
	}

	return prod, err
}

func handleRTSP(url string, cmd *exec.Cmd, path string) (core.Producer, error) {
	stderr := limitBuffer{buf: make([]byte, 512)}

	if cmd.Stderr != nil {
		cmd.Stderr = io.MultiWriter(cmd.Stderr, &stderr)
	} else {
		cmd.Stderr = &stderr
	}

	if log.Trace().Enabled() {
		cmd.Stdout = os.Stdout
	}

	waiter := make(chan core.Producer)

	waitersMu.Lock()
	waiters[path] = waiter
	waitersMu.Unlock()

	defer func() {
		waitersMu.Lock()
		delete(waiters, path)
		waitersMu.Unlock()
	}()

	log.Debug().Str("url", url).Str("cmd", fmt.Sprintf("%s", strings.Join(cmd.Args, " "))).Msg("[exec] run")

	ts := time.Now()

	if err := cmd.Start(); err != nil {
		log.Error().Err(err).Str("url", url).Msg("[exec]")
		return nil, err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(time.Second * 60):
		_ = cmd.Process.Kill()
		log.Error().Str("url", url).Msg("[exec] timeout")
		return nil, errors.New("timeout")
	case <-done:
		// limit message size
		return nil, errors.New("exec: " + stderr.String())
	case prod := <-waiter:
		log.Debug().Stringer("launch", time.Since(ts)).Msg("[exec] run")
		return prod, nil
	}
}

// internal

var (
	log       zerolog.Logger
	waiters   = map[string]chan core.Producer{}
	waitersMu sync.Mutex
)

type limitBuffer struct {
	buf []byte
	n   int
}

func (l *limitBuffer) String() string {
	if l.n == len(l.buf) {
		return string(l.buf) + "..."
	}
	return string(l.buf[:l.n])
}

func (l *limitBuffer) Write(p []byte) (int, error) {
	if l.n < cap(l.buf) {
		l.n += copy(l.buf[l.n:], p)
	}
	return len(p), nil
}
