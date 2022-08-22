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

func (r Response) String() string {
	s := r.Proto + " " + r.Status + EndLine
	for k, v := range r.Header {
		s += k + ": " + v[0] + EndLine
	}
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
	for k, v := range r.Header {
		s += k + ": " + v[0] + EndLine
	}
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
