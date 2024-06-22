package http

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hls"
	"github.com/AlexxIT/go2rtc/pkg/image"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	"github.com/AlexxIT/go2rtc/pkg/mpjpeg"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

func Init() {
	streams.HandleFunc("http", handleHTTP)
	streams.HandleFunc("https", handleHTTP)
	streams.HandleFunc("httpx", handleHTTP)

	streams.HandleFunc("tcp", handleTCP)

	api.HandleFunc("api/stream", apiStream)
}

func handleHTTP(rawURL string) (core.Producer, error) {
	rawURL, rawQuery, _ := strings.Cut(rawURL, "#")

	// first we get the Content-Type to define supported producer
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}

	if rawQuery != "" {
		query := streams.ParseQuery(rawQuery)

		for _, header := range query["header"] {
			key, value, _ := strings.Cut(header, ":")
			req.Header.Add(key, strings.TrimSpace(value))
		}
	}

	prod, err := do(req)
	if err != nil {
		return nil, err
	}

	if info, ok := prod.(core.Info); ok {
		info.SetProtocol("http")
		info.SetRemoteAddr(req.URL.Host) // TODO: rewrite to net.Conn
		info.SetURL(rawURL)
	}

	return prod, nil
}

func do(req *http.Request) (core.Producer, error) {
	res, err := tcp.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}

	// 1. Guess format from content type
	ct := res.Header.Get("Content-Type")
	if i := strings.IndexByte(ct, ';'); i > 0 {
		ct = ct[:i]
	}

	var ext string
	if i := strings.LastIndexByte(req.URL.Path, '.'); i > 0 {
		ext = req.URL.Path[i+1:]
	}

	switch {
	case ct == "application/vnd.apple.mpegurl" || ext == "m3u8":
		return hls.OpenURL(req.URL, res.Body)
	case ct == "image/jpeg":
		return image.Open(res)
	case ct == "multipart/x-mixed-replace":
		return mpjpeg.Open(res.Body)
	}

	return magic.Open(res.Body)
}

func handleTCP(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialTimeout("tcp", u.Host, core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}

	return magic.Open(conn)
}

func apiStream(w http.ResponseWriter, r *http.Request) {
	dst := r.URL.Query().Get("dst")
	stream := streams.Get(dst)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	client, err := magic.Open(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stream.AddProducer(client)
	defer stream.RemoveProducer(client)

	if err = client.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
