package debug

import (
	"bytes"
	"fmt"
	"github.com/AlexxIT/go2rtc/internal/api"
	"net/http"
	"runtime"
)

var stackSkip = [][]byte{
	// main.go
	[]byte("main.main()"),
	[]byte("created by os/signal.Notify"),

	// api/stack.go
	[]byte("github.com/AlexxIT/go2rtc/internal/api.stackHandler"),

	// api/api.go
	[]byte("created by github.com/AlexxIT/go2rtc/internal/api.Init"),
	[]byte("created by net/http.(*connReader).startBackgroundRead"),
	[]byte("created by net/http.(*Server).Serve"), // TODO: why two?

	[]byte("created by github.com/AlexxIT/go2rtc/internal/rtsp.Init"),
	[]byte("created by github.com/AlexxIT/go2rtc/internal/srtp.Init"),

	// webrtc/api.go
	[]byte("created by github.com/pion/ice/v2.NewTCPMuxDefault"),
	[]byte("created by github.com/pion/ice/v2.NewUDPMuxDefault"),
}

func stackHandler(w http.ResponseWriter, r *http.Request) {
	sep := []byte("\n\n")
	buf := make([]byte, 65535)
	i := 0
	n := runtime.Stack(buf, true)
	skipped := 0
	for _, item := range bytes.Split(buf[:n], sep) {
		for _, skip := range stackSkip {
			if bytes.Contains(item, skip) {
				item = nil
				skipped++
				break
			}
		}
		if item != nil {
			i += copy(buf[i:], item)
			i += copy(buf[i:], sep)
		}
	}
	i += copy(buf[i:], fmt.Sprintf(
		"Total: %d, Skipped: %d", runtime.NumGoroutine(), skipped),
	)

	api.ResponseText(w, buf[:i])
}
