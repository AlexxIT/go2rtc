package homekit

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CMAF error codes from the open-source guide (Camera Buffer Event CMAF Error)
const (
	CMAFErrNone                   = 0
	CMAFErrUnknown                = 1
	CMAFErrCannotFindHost         = 2
	CMAFErrCertConnectionFailure  = 3
	CMAFErrCannotCertify          = 4
	CMAFErrInvalidState           = 5
	CMAFErrRequiresRetry          = 6
	CMAFErrNoResponse             = 7
	CMAFErrMaxSessionTimeExceeded = 8
	CMAFErrCanceled               = 9
	CMAFErrMP4Error               = 10
	CMAFErrConnectionFailed       = 11
	CMAFErrTimeout                = 12
	CMAFErrOutOfResources         = 13
	CMAFErrInvalidData            = 14
	CMAFErrHTTPBadRequest         = 15
	CMAFErrHTTPInvalidToken       = 16
	CMAFErrHTTPCameraZoneDisabled = 17
	CMAFErrHTTPMismatchedToken    = 18
	CMAFErrHTTPNotFound           = 19
	CMAFErrHTTPInitMissing        = 20
	CMAFErrHTTPUnsupportedMedia   = 21
	CMAFErrHTTPBlocked            = 22
	CMAFErrHTTPCertificateExpired = 23
	CMAFErrHTTPInternalServer     = 24
	CMAFErrHTTPServiceUnavailable = 25
	CMAFErrHTTPZoneDoesNotExist   = 26
)

// CMAFClient publishes CMAF tracks to a DASH-IF Interface-1 publishing point over mTLS
type CMAFClient struct {
	baseURL    string
	httpClient *http.Client
	userAgent  string
}

// NewCMAFClient builds a client for baseURL (must end with /)
// tlsCfg may be nil for plain HTTP (local tests only)
func NewCMAFClient(baseURL string, tlsCfg *tls.Config) *CMAFClient {
	if baseURL != "" && !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
		TLSClientConfig:     tlsCfg,
		ForceAttemptHTTP2:   true,
	}
	return &CMAFClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   60 * time.Second,
		},
		userAgent: "DASH-IF-Ingest/1.1 go2rtc-hksv",
	}
}

// PublishClip uploads init + media fragments for video (and audio if present)
// Paths follow a simple RepresentationID scheme under the publishing point
func (c *CMAFClient) PublishClip(sessionID uint64, clip *Clip) (cmafErr int, err error) {
	if c.baseURL == "" {
		return CMAFErrInvalidState, errString("homekit: publishing point not set")
	}
	if clip == nil || len(clip.Init) == 0 {
		return CMAFErrMP4Error, errString("homekit: invalid clip")
	}

	// Connectivity probe (empty POST) is recommended by DASH-IF; ignore soft failures
	_ = c.post(c.baseURL, nil, "application/octet-stream")

	videoRep := fmt.Sprintf("video-%d", sessionID)
	if err := c.postInit(videoRep, clip.Init); err != nil {
		return mapHTTPError(err), err
	}

	for i, frag := range clip.Fragments {
		if err := c.postMedia(videoRep, uint64(i+1), frag); err != nil {
			return mapHTTPError(err), err
		}
	}
	return CMAFErrNone, nil
}

func (c *CMAFClient) postInit(rep string, body []byte) error {
	// DASH-IF: one complete CMAF object per request
	u := c.baseURL + rep + "/init.mp4"
	return c.post(u, body, "video/mp4")
}

func (c *CMAFClient) postMedia(rep string, number uint64, body []byte) error {
	u := c.baseURL + fmt.Sprintf("%s/seg-%d.m4s", rep, number)
	return c.post(u, body, "video/mp4")
}

func (c *CMAFClient) post(rawURL string, body []byte, contentType string) error {
	if _, err := url.Parse(rawURL); err != nil {
		return err
	}
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	} else {
		rdr = http.NoBody
	}
	req, err := http.NewRequest(http.MethodPost, rawURL, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if body != nil {
		req.ContentLength = int64(len(body))
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	return &httpStatusError{code: res.StatusCode, url: rawURL}
}

type httpStatusError struct {
	code int
	url  string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("homekit: CMAF POST %s -> HTTP %d", e.url, e.code)
}

func mapHTTPError(err error) int {
	if err == nil {
		return CMAFErrNone
	}
	if se, ok := err.(*httpStatusError); ok {
		switch se.code {
		case http.StatusBadRequest:
			return CMAFErrHTTPBadRequest
		case http.StatusUnauthorized, http.StatusForbidden:
			return CMAFErrHTTPInvalidToken
		case http.StatusNotFound:
			return CMAFErrHTTPNotFound
		case http.StatusUnsupportedMediaType:
			return CMAFErrHTTPUnsupportedMedia
		case http.StatusPreconditionFailed:
			return CMAFErrHTTPInitMissing
		case http.StatusInternalServerError:
			return CMAFErrHTTPInternalServer
		case http.StatusServiceUnavailable:
			return CMAFErrHTTPServiceUnavailable
		case 419: // some stacks use for cert expired
			return CMAFErrHTTPCertificateExpired
		default:
			if se.code >= 500 {
				return CMAFErrHTTPInternalServer
			}
			return CMAFErrUnknown
		}
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no such host"):
		return CMAFErrCannotFindHost
	case strings.Contains(msg, "certificate"):
		return CMAFErrCertConnectionFailure
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "Timeout"):
		return CMAFErrTimeout
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "connection reset"):
		return CMAFErrConnectionFailed
	default:
		return CMAFErrUnknown
	}
}
