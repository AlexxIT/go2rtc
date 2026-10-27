package tcp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"net/url"
	"sort"
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

func (r Response) String() string {
	s := r.Proto + " " + r.Status + EndLine
	s += headerString(r.Header)
	s += EndLine
	if r.Body != nil {
		s += string(r.Body)
	}
	return s
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
		res.Body = make([]byte, i)
		if _, err = io.ReadAtLeast(r, res.Body, i); err != nil {
			return nil, err
		}
	}

	return res, nil
}

// headerString writes CSeq first and the other headers in sorted order.
// Some servers (e.g. Google Nest RTSP, Wowza) ignore requests unless CSeq
// is the first header, and ranging over the map gives a random order.
// https://github.com/AlexxIT/go2rtc/issues/2622
func headerString(header textproto.MIMEHeader) string {
	var s string
	keys := make([]string, 0, len(header))
	for k := range header {
		if strings.EqualFold(k, "CSeq") {
			if v := header[k]; len(v) > 0 {
				s += k + ": " + v[0] + EndLine
			}
		} else {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v := header[k]; len(v) > 0 {
			s += k + ": " + v[0] + EndLine
		}
	}
	return s
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
	s := r.Method + " " + r.URL.String() + " " + r.Proto + EndLine
	s += headerString(r.Header)
	s += EndLine
	if r.Body != nil {
		s += string(r.Body)
	}
	return s
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
		req.Body = make([]byte, i)
		if _, err = io.ReadAtLeast(r, req.Body, i); err != nil {
			return nil, err
		}
	}

	return req, nil
}
