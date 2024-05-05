package hls

import (
	"bytes"
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

	playlist    []byte
	lastSegment []byte
	lastTime    time.Time

	buf []byte
}

func NewReader(u *url.URL, body io.ReadCloser) (io.Reader, error) {
	b, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	var rawURL string

	re := regexp.MustCompile(`#EXT-X-STREAM-INF.+?\n(\S+)`)
	m := re.FindSubmatch(b)
	if m != nil {
		ref, err := url.Parse(string(m[1]))
		if err != nil {
			return nil, err
		}

		rawURL = u.ResolveReference(ref).String()
	} else {
		rawURL = u.String()
	}

	req, err := http.NewRequest("GET", rawURL, nil)
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

func (r *reader) Close() error {
	r.client.Transport = r // after close we fail on next request
	return nil
}

func (r *reader) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, io.EOF
}

func (r *reader) getSegment() ([]byte, error) {
	for i := 0; i < 10; i++ {
		if r.playlist == nil {
			if wait := time.Second - time.Since(r.lastTime); wait > 0 {
				time.Sleep(wait)
			}

			// 1. Load playlist
			res, err := r.client.Do(r.request)
			if err != nil {
				return nil, err
			}

			r.playlist, err = io.ReadAll(res.Body)
			if err != nil {
				return nil, err
			}

			r.lastTime = time.Now()

			//log.Printf("[hls] load playlist\n%s", r.playlist)
		}

		for r.playlist != nil {
			// 2. Remove all previous segments from playlist
			if i := bytes.Index(r.playlist, r.lastSegment); i > 0 {
				r.playlist = r.playlist[i:]
			}

			// 3. Get link to new segment
			segment := getSegment(r.playlist)
			if segment == nil {
				r.playlist = nil
				break
			}

			//log.Printf("[hls] load segment: %s", segment)

			ref, err := url.Parse(string(segment))
			if err != nil {
				return nil, err
			}

			ref = r.request.URL.ResolveReference(ref)
			res, err := r.client.Get(ref.String())
			if err != nil {
				return nil, err
			}

			r.lastSegment = segment

			return io.ReadAll(res.Body)
		}
	}

	return nil, io.EOF
}

func getSegment(src []byte) []byte {
	for ok := false; !ok; {
		ok = bytes.HasPrefix(src, []byte("#EXTINF"))

		i := bytes.IndexByte(src, '\n') + 1
		if i == 0 {
			return nil
		}

		src = src[i:]
	}

	if i := bytes.IndexByte(src, '\n'); i > 0 {
		return src[:i]
	}

	return src
}
