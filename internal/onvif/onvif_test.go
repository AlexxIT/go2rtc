package onvif

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/onvif"
	"github.com/stretchr/testify/require"
)

func TestMediaURIEscapesStreamName(t *testing.T) {
	prev := rtsp.Port
	rtsp.Port = "8554"
	t.Cleanup(func() { rtsp.Port = prev })

	request := func(operation, token string) string {
		body := `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body>` +
			`<trt:` + operation + ` xmlns:trt="http://www.onvif.org/ver10/media/wsdl">` +
			`<trt:ProfileToken>` + token + `</trt:ProfileToken></trt:` + operation + `></s:Body></s:Envelope>`
		r := httptest.NewRequest("POST", "/onvif/device_service", strings.NewReader(body))
		r.Host = "192.168.1.10:1984"
		w := httptest.NewRecorder()
		onvifDeviceService(w, r)
		return onvif.FindTagValue(w.Body.Bytes(), "Uri")
	}

	// Home Assistant names streams like camera_<percent-encoded unique id>
	name := "camera_FD%3A53"
	require.Equal(t, "rtsp://192.168.1.10:8554/camera_FD%253A53", request(onvif.MediaGetStreamUri, name))
	require.Equal(t, "http://192.168.1.10:1984/api/frame.jpeg?src=camera_FD%253A53", request(onvif.MediaGetSnapshotUri, name))

	require.Equal(t, "rtsp://192.168.1.10:8554/camera1", request(onvif.MediaGetStreamUri, "camera1"))
	require.Equal(t, "http://192.168.1.10:1984/api/frame.jpeg?src=camera1", request(onvif.MediaGetSnapshotUri, "camera1"))
}
