package rtsp

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestURLParse(t *testing.T) {
	base := "rtsp://::ffff:192.168.1.123/onvif/profile.1/"
	_, err := urlParse(base)
	assert.Empty(t, err)
}
