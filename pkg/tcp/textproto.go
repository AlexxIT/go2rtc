package tcp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
)

const EndLine = "\r\n"

// Response like http.Response, but with any proto
type Response struct {
	Status     string
	StatusCode int
	Proto      string
	Header     textproto.MIMEHeader
	Body       []byte
	Request    *Request
}

func (r *Response) String() string {
	var s strings.Builder
	s.WriteString(r.Proto + " " + r.Status + EndLine)
	writeHeaders(&s, r.Header)
	s.WriteString(EndLine)
	s.Write(r.Body)
	return s.String()
}

func (r *Response) Write(w io.Writer) (err error) {
	_, err = w.Write([]byte(r.String()))
	return
}

func ReadResponse(r *bufio.Reader) (*Response, error) {
	tp := textproto.NewReader(r)

	line, err := tp.ReadLine()
	if err != nil {
		return nil, err
	}
	if line == "" {
		return nil, errors.New("empty response on RTSP request")
	}

	ss := strings.SplitN(line, " ", 3)
	if len(ss) != 3 {
		return nil, fmt.Errorf("malformed response: %s", line)
	}

	res := &Response{
		Status: ss[1] + " " + ss[2],
		Proto:  ss[0],
	}

	res.StatusCode, err = strconv.Atoi(ss[1])
	if err != nil {
		return nil, err
	}

	res.Header, err = tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}

	if val := res.Header.Get("Content-Length"); val != "" {
		var i int
		i, err = strconv.Atoi(val)
		if err != nil {
			return nil, err
		}
		res.Body = make([]byte, i)
		if _, err = io.ReadAtLeast(r, res.Body, i); err != nil {
			return nil, err
		}
	}

	return res, nil
}

// writeHeaders follows the RTSP header order used by LIVE555 (VLC).
// Some servers (e.g. Google Nest RTSP, Wowza) are sensitive to header order.
// https://github.com/AlexxIT/go2rtc/issues/2622
func writeHeaders(s *strings.Builder, header textproto.MIMEHeader) {
	for _, k := range []string{
		"CSeq",
		"Authorization",
		"User-Agent",
		"Accept",
		"Content-Type",
		"Range",
		"Transport",
		"Require",
		"Session",
		"Server",
		"Public",
		"Www-Authenticate",
		"Content-Length",
	} {
		if v := header[k]; len(v) != 0 {
			s.WriteString(k + ": " + v[0] + EndLine)
		}
	}
}

// Request like http.Request, but with any proto
type Request struct {
	Method string
	URL    *url.URL
	Proto  string
	Header textproto.MIMEHeader
	Body   []byte
}

func (r *Request) String() string {
	var s strings.Builder
	s.WriteString(r.Method + " " + r.URL.String() + " " + r.Proto + EndLine)
	writeHeaders(&s, r.Header)
	s.WriteString(EndLine)
	s.Write(r.Body)
	return s.String()
}

func (r *Request) Write(w io.Writer) (err error) {
	_, err = w.Write([]byte(r.String()))
	return
}

func ReadRequest(r *bufio.Reader) (*Request, error) {
	tp := textproto.NewReader(r)

	line, err := tp.ReadLine()
	if err != nil {
		return nil, err
	}

	ss := strings.SplitN(line, " ", 3)
	if len(ss) != 3 {
		return nil, fmt.Errorf("wrong request: %s", line)
	}

	req := &Request{
		Method: ss[0],
		Proto:  ss[2],
	}

	req.URL, err = url.Parse(ss[1])
	if err != nil {
		return nil, err
	}

	req.Header, err = tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}

	if val := req.Header.Get("Content-Length"); val != "" {
		var i int
		i, err = strconv.Atoi(val)
		if err != nil {
			return nil, err
		}
		req.Body = make([]byte, i)
		if _, err = io.ReadAtLeast(r, req.Body, i); err != nil {
			return nil, err
		}
	}

	return req, nil
}
