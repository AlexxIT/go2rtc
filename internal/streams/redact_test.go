package streams

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedact(t *testing.T) {
	assert := assert.New(t)

	// Init fake redirect.
	RedirectFunc("fakeffmpeg", func(url string) (string, error) { return url[7:], nil })

	// No sensitive information
	assert.Equal("not_a_url", Redact("not_a_url"))
	assert.Equal("rtsp://localhost:8554", Redact("rtsp://localhost:8554"))

	// User + password
	assert.Equal(
		"rtsp://user:xxxxx@localhost:8554",
		Redact("rtsp://user:password@localhost:8554"),
	)

	// Only password
	assert.Equal(
		"rtsp://:xxxxx@localhost:8554",
		Redact("rtsp://:password@localhost:8554"),
	)

	// With configured redirect
	assert.Equal(
		"fakeffmpeg:rtsp://:xxxxx@localhost:8554",
		Redact("fakeffmpeg:rtsp://:password@localhost:8554"),
	)

	// Header option
	assert.Equal(
		"https://mjpeg.sanford.io/count.mjpeg#header=xxxxx",
		Redact("https://mjpeg.sanford.io/count.mjpeg#header=Authorization: Bearer XXX"),
	)

	// Token query parameter
	assert.Equal(
		"hass://192.168.1.123:8123?entity_id=camera.nest_doorbell&token=xxxxx",
		Redact("hass://192.168.1.123:8123?entity_id=camera.nest_doorbell&token=the-token"),
	)

	// OAuth credentials
	assert.Equal(
		"nest:?client_id=client-id&client_secret=xxxxx&refresh_token=xxxxx",
		Redact("nest:?client_id=client-id&client_secret=client-secret&refresh_token=refresh-token"),
	)

	// Redirect + password + query parameters + options
	assert.Equal(
		"fakeffmpeg:rtsp://:xxxxx@localhost:8554?foo=bar&token=xxxxx#refresh_token=xxxxx#timeout=30",
		Redact("fakeffmpeg:rtsp://:password@localhost:8554?foo=bar&token=foo#timeout=30#refresh_token=baz"),
	)

	// Exec
	assert.Equal(
		"exec:some-command --stream rtsp://user:xxxxx@localhost:8554 --name 'my stream'",
		Redact("exec: some-command --stream rtsp://user:password@localhost:8554 --name 'my stream'"),
	)
}
