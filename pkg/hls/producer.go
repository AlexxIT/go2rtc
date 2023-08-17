package hls

import (
	"io"
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
)

func OpenURL(u *url.URL, body io.ReadCloser) (core.Producer, error) {
	rd, err := NewReader(u, body)
	if err != nil {
		return nil, err
	}
	return mpegts.Open(rd)
}
