package hap

import (
	"errors"
	"io"
	"net/http"
	"time"
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
	if err := c.conn.SetWriteDeadline(time.Now().Add(ConnDeadline)); err != nil {
		return nil, err
	}
	if err := req.Write(c.conn); err != nil {
		return nil, err
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
