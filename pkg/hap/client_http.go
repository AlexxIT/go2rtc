package hap

import (
	"bufio"
	"errors"
	"io"
	"net/http"
)

const (
	MimeTLV8 = "application/pairing+tlv8"
	MimeJSON = "application/hap+json"

	PathPairSetup       = "/pair-setup"
	PathPairVerify      = "/pair-verify"
	PathPairings        = "/pairings"
	PathAccessories     = "/accessories"
	PathCharacteristics = "/characteristics"
	PathResource        = "/resource"
)

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if err := req.Write(c.Conn); err != nil {
		return nil, err
	}
	if c.res != nil {
		return <-c.res, c.err
	}
	return http.ReadResponse(c.reader, req)
}

func (c *Client) Request(method, path, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, "http://"+c.DeviceAddress+path, body)
	if err != nil {
		return nil, err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	res, err := c.Do(req)
	if err == nil && res.StatusCode >= http.StatusBadRequest {
		err = errors.New("hap: wrong http status: " + res.Status)
	}

	return res, err
}

func (c *Client) Get(path string) (*http.Response, error) {
	return c.Request("GET", path, "", nil)
}

func (c *Client) Post(path, contentType string, body io.Reader) (*http.Response, error) {
	return c.Request("POST", path, contentType, body)
}

func (c *Client) Put(path, contentType string, body io.Reader) (*http.Response, error) {
	return c.Request("PUT", path, contentType, body)
}

const ProtoEvent = "EVENT/1.0"

func ReadResponse(r *bufio.Reader, req *http.Request) (*http.Response, error) {
	b, err := r.Peek(9)
	if err != nil {
		return nil, err
	}

	if string(b) != ProtoEvent {
		return http.ReadResponse(r, req)
	}

	copy(b, "HTTP/1.1 ")

	res, err := http.ReadResponse(r, req)
	if err != nil {
		return nil, err
	}

	res.Proto = ProtoEvent

	return res, nil
}
