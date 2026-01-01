package mpjpeg

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
)

func Next(rd *bufio.Reader) (http.Header, []byte, error) {
	for {
		// search next boundary and skip empty lines
		s, err := rd.ReadString('\n')
		if err != nil {
			return nil, nil, err
		}

		if s == "\r\n" {
			continue
		}

		if !strings.HasPrefix(s, "--") {
			return nil, nil, errors.New("multipart: wrong boundary: " + s)
		}

		// Foscam G2 has a awful implementation of MJPEG
		// https://github.com/AlexxIT/go2rtc/issues/1258
		if b, _ := rd.Peek(2); string(b) == "--" {
			continue
		}

		break
	}

	tp := textproto.NewReader(rd)
	header, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, nil, err
	}

	s := header.Get("Content-Length")
	if s == "" {
		return nil, nil, errors.New("multipart: no content length")
	}

	size, err := strconv.Atoi(s)
	if err != nil {
		return nil, nil, err
	}

	buf := make([]byte, size)
	if _, err = io.ReadFull(rd, buf); err != nil {
		return nil, nil, err
	}

	return http.Header(header), buf, nil
}
