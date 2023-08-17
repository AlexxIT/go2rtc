package hls

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type reader struct {
	client  *http.Client
	request *http.Request

	lastSegment []byte
	lastTime    time.Time

	buf []byte
}

func NewReader(u *url.URL, body io.ReadCloser) (io.Reader, error) {
	b, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`#EXT-X-STREAM-INF.+?\n(\S+)`)
	m := re.FindSubmatch(b)
	if m == nil {
		return nil, errors.New("hls: wrong playlist: " + string(b))
	}

	ref, err := url.Parse(string(m[1]))
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", u.ResolveReference(ref).String(), nil)
	if err != nil {
		return nil, err
	}

	rd := &reader{
		client:  &http.Client{Timeout: core.ConnDialTimeout},
		request: req,
	}
	return rd, nil
}

func (r *reader) Read(dst []byte) (n int, err error) {
	// 1. Check temporary tempbuffer
	if len(r.buf) == 0 {
		src, err2 := r.getSegment()
		if err2 != nil {
			return 0, err2
		}

		// 2. Check if the message fits in the buffer
		if len(src) <= len(dst) {
			return copy(dst, src), nil
		}

		// 3. Put the message into a temporary buffer
		r.buf = src
	}

	// 4. Send temporary buffer
	n = copy(dst, r.buf)
	r.buf = r.buf[n:]
	return
}

func (r *reader) getSegment() ([]byte, error) {
	for {
		if wait := time.Second - time.Since(r.lastTime); wait > 0 {
			time.Sleep(wait)
		}

		// 1. Load playlist
		res, err := r.client.Do(r.request)
		if err != nil {
			return nil, err
		}

		playlist, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}

		r.lastTime = time.Now()

		//log.Printf("[hls] load playlist\n%s", playlist)

		// 2. Remove all previous segments from playlist
		if i := bytes.Index(playlist, r.lastSegment); i > 0 {
			playlist = playlist[i:]
		}

		for playlist != nil {
			// 3. Get link to new segment
			var segment []byte
			if segment, playlist = getSegment(playlist); segment == nil {
				break
			}

			//log.Printf("[hls] load segment: %s", segment)

			ref, err2 := url.Parse(string(segment))
			if err2 != nil {
				return nil, err2
			}

			ref = r.request.URL.ResolveReference(ref)
			if res, err2 = r.client.Get(ref.String()); err2 != nil {
				return nil, err2
			}

			r.lastSegment = segment

			return io.ReadAll(res.Body)
		}
	}
}

func getSegment(src []byte) (segment, left []byte) {
	for ok := false; !ok; {
		ok = bytes.HasPrefix(src, []byte("#EXTINF"))

		i := bytes.IndexByte(src, '\n') + 1
		if i == 0 {
			return nil, nil
		}

		src = src[i:]
	}

	if i := bytes.IndexByte(src, '\n'); i > 0 {
		return src[:i], src[i+1:]
	}

	return src, nil
}
