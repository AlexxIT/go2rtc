package hap

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strconv"
)

const (
	MimeTLV8 = "application/pairing+tlv8"
	MimeJSON = "application/hap+json"

	UriPairSetup       = "/pair-setup"
	UriPairVerify      = "/pair-verify"
	UriPairings        = "/pairings"
	UriAccessories     = "/accessories"
	UriCharacteristics = "/characteristics"
	UriResource        = "/resource"
)

func (c *Conn) Write(p []byte) (r io.Reader, err error) {
	if c.secure == nil {
		if _, err = c.conn.Write(p); err == nil {
			r = bufio.NewReader(c.conn)
		}
	} else {
		if _, err = c.secure.Write(p); err == nil {
			r = <-c.httpResponse
		}
	}
	return
}

func (c *Conn) Do(req *http.Request) (*http.Response, error) {
	if c.secure == nil {
		// insecure requests
		if err := req.Write(c.conn); err != nil {
			return nil, err
		}
		return http.ReadResponse(bufio.NewReader(c.conn), req)
	}

	// secure support write interface to connection
	if err := req.Write(c.secure); err != nil {
		return nil, err
	}

	// get decrypted buffer from connection
	buf := <-c.httpResponse

	return http.ReadResponse(buf, req)
}

func (c *Conn) Get(uri string) (*http.Response, error) {
	req, err := http.NewRequest(
		"GET", "http://"+c.DeviceAddress+uri, nil,
	)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func (c *Conn) Post(uri string, data []byte) (*http.Response, error) {
	req, err := http.NewRequest(
		"POST", "http://"+c.DeviceAddress+uri,
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, err
	}

	switch uri {
	case "/pair-verify", "/pairings":
		req.Header.Set("Content-Type", MimeTLV8)
	case UriResource:
		req.Header.Set("Content-Type", MimeJSON)
	}

	return c.Do(req)
}

func (c *Conn) Put(uri string, data []byte) (*http.Response, error) {
	req, err := http.NewRequest(
		"PUT", "http://"+c.DeviceAddress+uri,
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, err
	}

	switch uri {
	case UriCharacteristics:
		req.Header.Set("Content-Type", MimeJSON)
	}

	return c.Do(req)
}

func (c *Conn) Handle() (err error) {
	defer func() {
		if c.conn == nil {
			err = nil
		}
	}()

	b := make([]byte, 512000)
	for {
		var total, content int
		header := -1

		for {
			var n1 int
			n1, err = c.secure.Read(b[total:])
			if err != nil {
				return err
			}

			if n1 == 0 {
				return io.EOF
			}

			total += n1

			// TODO: rewrite
			if header == -1 {
				// step 1. wait whole header
				header = bytes.Index(b[:total], []byte("\r\n\r\n"))
				if header < 0 {
					continue
				}
				header += 4

				// step 2. check content-length
				i1 := bytes.Index(b[:total], []byte("Content-Length: "))
				if i1 < 0 {
					break
				}
				i1 += 16
				i2 := bytes.IndexByte(b[i1:total], '\r')
				content, err = strconv.Atoi(string(b[i1 : i1+i2]))
				if err != nil {
					break
				}
			}

			if total >= header+content {
				break
			}
		}

		// copy slice to buffer
		buf := bytes.NewBuffer(make([]byte, 0, total))
		buf.Write(b[:total])
		r := bufio.NewReader(buf)

		// EVENT/1.0 200 OK
		if b[0] == 'E' {
			if c.OnEvent == nil {
				continue
			}

			tp := textproto.NewReader(r)

			var s string
			if s, err = tp.ReadLine(); err != nil {
				return err
			}
			if s != "EVENT/1.0 200 OK" {
				return errors.New("wrong response")
			}

			var mimeHeader textproto.MIMEHeader
			if mimeHeader, err = tp.ReadMIMEHeader(); err != nil {
				return err
			}

			var cl int
			if cl, err = strconv.Atoi(
				mimeHeader.Get("Content-Length"),
			); err != nil {
				return err
			}

			res := http.Response{
				StatusCode:    200,
				Proto:         "EVENT/1.0",
				ProtoMajor:    1,
				ProtoMinor:    0,
				Header:        http.Header(mimeHeader),
				ContentLength: int64(cl),
				Body:          io.NopCloser(r),
			}
			c.OnEvent(&res)
			continue
		}

		//if bytes.Index(b, []byte("image/jpeg")) > 0 {
		//	if total, err = c.secure.Read(b); err != nil {
		//		return
		//	}
		//	buf.Write(b[:total])
		//}

		c.httpResponse <- r
	}
}

func WriteStatusCode(w io.Writer, statusCode int) (err error) {
	body := []byte(fmt.Sprintf(
		"HTTP/1.1 %d %s\n\n", statusCode, http.StatusText(statusCode),
	))
	//print("<<<", string(body), "<<<\n")
	_, err = w.Write(body)
	return
}

func WriteResponse(
	w io.Writer, statusCode int, contentType string, body []byte,
) (err error) {
	header := fmt.Sprintf(
		"HTTP/1.1 %d %s\nContent-Type: %s\nContent-Length: %d\n\n",
		statusCode, http.StatusText(statusCode), contentType, len(body),
	)
	body = append([]byte(header), body...)
	//print("<<<", string(body), "<<<\n")
	_, err = w.Write(body)
	return
}

func WriteChunked(w io.Writer, contentType string, body []byte) (err error) {
	header := fmt.Sprintf(
		"HTTP/1.1 200 OK\nContent-Type: %s\nTransfer-Encoding: chunked\n\n%x\n",
		contentType, len(body),
	)
	body = append([]byte(header), body...)
	body = append(body, "\n0\n\n"...)
	//print("<<<", string(body), "<<<\n")
	_, err = w.Write(body)
	return
}
