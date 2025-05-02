package exec

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	pkg "github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/shell"
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

func execHandle(rawURL string) (prod core.Producer, err error) {
	rawURL, rawQuery, _ := strings.Cut(rawURL, "#")
	query := streams.ParseQuery(rawQuery)

	var path string

	// RTSP flow should have `{output}` inside URL
	// pipe flow may have `#{params}` inside URL
	if i := strings.Index(rawURL, "{output}"); i > 0 {
		if rtsp.Port == "" {
			return nil, errors.New("exec: rtsp module disabled")
		}

		sum := md5.Sum([]byte(rawURL))
		path = "/" + hex.EncodeToString(sum[:])
		rawURL = rawURL[:i] + "rtsp://127.0.0.1:" + rtsp.Port + path + rawURL[i+8:]
	}

	cmd := shell.NewCommand(rawURL[5:]) // remove `exec:`
	cmd.Stderr = &logWriter{
		buf:   make([]byte, 512),
		debug: log.Debug().Enabled(),
	}

	if s := query.Get("killsignal"); s != "" {
		sig := syscall.Signal(core.Atoi(s))
		cmd.Cancel = func() error {
			log.Debug().Msgf("[exec] kill with signal=%d", sig)
			return cmd.Process.Signal(sig)
		}
	}

	if s := query.Get("killtimeout"); s != "" {
		cmd.WaitDelay = time.Duration(core.Atoi(s)) * time.Second
	}

	if query.Get("backchannel") == "1" {
		return pcm.NewBackchannel(cmd, query.Get("audio"))
	}

	if path == "" {
		prod, err = handlePipe(rawURL, cmd)
	} else {
		prod, err = handleRTSP(rawURL, cmd, path)
	}

	if err != nil {
		_ = cmd.Close()
	}

	return
}

func handlePipe(source string, cmd *shell.Command) (core.Producer, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	rd := struct {
		io.Reader
		io.Closer
	}{
		// add buffer for pipe reader to reduce syscall
		bufio.NewReaderSize(stdout, core.BufferSize),
		// stop cmd on close pipe call
		cmd,
	}

	log.Debug().Strs("args", cmd.Args).Msg("[exec] run pipe")

	ts := time.Now()

	if err = cmd.Start(); err != nil {
		return nil, err
	}

	prod, err := magic.Open(rd)
	if err != nil {
		return nil, fmt.Errorf("exec/pipe: %w\n%s", err, cmd.Stderr)
	}

	if info, ok := prod.(core.Info); ok {
		info.SetProtocol("pipe")
		setRemoteInfo(info, source, cmd.Args)
	}

	log.Debug().Stringer("launch", time.Since(ts)).Msg("[exec] run pipe")

	return prod, nil
}

func handleRTSP(source string, cmd *shell.Command, path string) (core.Producer, error) {
	if log.Trace().Enabled() {
		cmd.Stdout = os.Stdout
	}

	waiter := make(chan *pkg.Conn, 1)

	waitersMu.Lock()
	waiters[path] = waiter
	waitersMu.Unlock()

	defer func() {
		waitersMu.Lock()
		delete(waiters, path)
		waitersMu.Unlock()
	}()

	log.Debug().Strs("args", cmd.Args).Msg("[exec] run rtsp")

	ts := time.Now()

	if err := cmd.Start(); err != nil {
		log.Error().Err(err).Str("source", source).Msg("[exec]")
		return nil, err
	}

	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	select {
	case <-timeout.C:
		// haven't received data from app in timeout
		log.Error().Str("source", source).Msg("[exec] timeout")
		return nil, errors.New("exec: timeout")
	case <-cmd.Done():
		// app fail before we receive any data
		return nil, fmt.Errorf("exec/rtsp\n%s", cmd.Stderr)
	case prod := <-waiter:
		// app started successfully
		log.Debug().Stringer("launch", time.Since(ts)).Msg("[exec] run rtsp")
		setRemoteInfo(prod, source, cmd.Args)
		prod.OnClose = cmd.Close
		return prod, nil
	}
}

// internal

var (
	log       zerolog.Logger
	waiters   = make(map[string]chan *pkg.Conn)
	waitersMu sync.Mutex
)

type logWriter struct {
	buf   []byte
	debug bool
	n     int
}

func (l *logWriter) String() string {
	if l.n == len(l.buf) {
		return string(l.buf) + "..."
	}
	return string(l.buf[:l.n])
}

func (l *logWriter) Write(p []byte) (n int, err error) {
	if l.n < cap(l.buf) {
		l.n += copy(l.buf[l.n:], p)
	}
	n = len(p)
	if l.debug {
		if p = trimSpace(p); p != nil {
			log.Debug().Msgf("[exec] %s", p)
		}
	}
	return
}

func trimSpace(b []byte) []byte {
	start := 0
	stop := len(b)
	for ; start < stop; start++ {
		if b[start] >= ' ' {
			break // trim all ASCII before 0x20
		}
	}
	for ; ; stop-- {
		if stop == start {
			return nil // skip empty output
		}
		if b[stop-1] > ' ' {
			break // trim all ASCII before 0x21
		}
	}
	return b[start:stop]
}

func setRemoteInfo(info core.Info, source string, args []string) {
	info.SetSource(source)

	if i := core.Index(args, "-i"); i > 0 && i < len(args)-1 {
		rawURL := args[i+1]
		if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
			info.SetRemoteAddr(u.Host)
			info.SetURL(rawURL)
		}
	}
}
