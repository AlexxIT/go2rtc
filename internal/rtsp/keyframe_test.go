package rtsp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/stretchr/testify/require"
)

var onvifOnce sync.Once
var onvifSync = make(chan string, 10)

// onvifServer - fake ONVIF camera with two profiles, sends SetSynchronizationPoint tokens to onvifSync
func onvifServer() {
	re := regexp.MustCompile(`ProfileToken>([^<]+)`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var token string
		if m := re.FindSubmatch(b); m != nil {
			token = string(m[1])
		}

		switch body := string(b); {
		case strings.Contains(body, "GetCapabilities"):
			_, _ = io.WriteString(w, `<tt:Media><tt:XAddr>http://`+r.Host+`/onvif/media_service</tt:XAddr></tt:Media>`)
		case strings.Contains(body, "GetProfiles"):
			_, _ = io.WriteString(w, `<trt:Profiles token="000"></trt:Profiles><trt:Profiles token="001"></trt:Profiles>`)
		case strings.Contains(body, "GetStreamUri"):
			path := map[string]string{"000": "main", "001": "sub"}[token]
			_, _ = io.WriteString(w, `<tt:Uri>rtsp://127.0.0.1:554/`+path+`</tt:Uri>`)
		case strings.Contains(body, "SetSynchronizationPoint"):
			onvifSync <- token
		}
	}))

	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	keyframePorts = []int{1, port} // first port is closed
}

func TestKeyframe(t *testing.T) {
	onvifOnce.Do(onvifServer)

	keyframeMu.Lock()
	keyframeTargets = map[string]*keyframeTarget{}
	keyframeMu.Unlock()

	rawURL := "rtsp://admin:pass@127.0.0.1:554/sub"
	conn := rtsp.NewClient(rawURL)
	listenKeyframe(conn, rawURL)

	// no request for responses to other methods
	conn.Fire(&tcp.Request{Method: rtsp.MethodSetup})
	conn.Fire(&tcp.Response{StatusCode: http.StatusOK})
	conn.Fire(&tcp.Request{Method: rtsp.MethodPlay})
	conn.Fire(&tcp.Response{StatusCode: http.StatusOK})

	select {
	case token := <-onvifSync:
		require.Equal(t, "001", token)
	case <-time.After(5 * time.Second):
		require.Fail(t, "no keyframe request")
	}

	select {
	case <-onvifSync:
		require.Fail(t, "only one keyframe request")
	case <-time.After(100 * time.Millisecond):
	}
}
