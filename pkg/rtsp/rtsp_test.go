package rtsp

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestURLParse(t *testing.T) {
	// https://github.com/AlexxIT/WebRTC/issues/395
	base := "rtsp://::ffff:192.168.1.123/onvif/profile.1/"
	u, err := urlParse(base)
	assert.Empty(t, err)
	assert.Equal(t, "::ffff:192.168.1.123:", u.Host)

	// https://github.com/AlexxIT/go2rtc/issues/208
	base = "rtsp://rtsp://turret2-cam.lan:554/stream1/"
	u, err = urlParse(base)
	assert.Empty(t, err)
	assert.Equal(t, "turret2-cam.lan:554", u.Host)
}
