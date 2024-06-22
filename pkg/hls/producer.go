package hls

import (
	"io"
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/mpegts"
)

func OpenURL(u *url.URL, body io.ReadCloser) (*mpegts.Producer, error) {
	rd, err := NewReader(u, body)
	if err != nil {
		return nil, err
	}
	prod, err := mpegts.Open(rd)
	if err != nil {
		return nil, err
	}
	prod.FormatName = "hls/mpegts"
	prod.RemoteAddr = u.Host
	return prod, nil
}
