package onvif

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const PathDevice = "/onvif/device_service"

type Client struct {
	url *url.URL

	deviceURL string
	mediaURL  string
	imaginURL string
}

func NewClient(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	baseURL := "http://" + u.Host

	client := &Client{url: u}
	client.deviceURL = baseURL + GetPath(u.Path, PathDevice)

	b, err := client.DeviceRequest(DeviceGetCapabilities)
	if err != nil {
		return nil, err
	}

	s := FindTagValue(b, "Media.+?XAddr")
	client.mediaURL = baseURL + GetPath(s, "/onvif/media_service")

	s = FindTagValue(b, "Imaging.+?XAddr")
	client.imaginURL = baseURL + GetPath(s, "/onvif/imaging_service")

	return client, nil
}

func (c *Client) GetURI() (string, error) {
	query := c.url.Query()

	token := query.Get("subtype")

	// support empty
	if i := atoi(token); i >= 0 {
		tokens, err := c.GetProfilesTokens()
		if err != nil {
			return "", err
		}
		if i >= len(tokens) {
			return "", errors.New("onvif: wrong subtype")
		}
		token = tokens[i]
	}

	getUri := c.GetStreamUri
	if query.Has("snapshot") {
		getUri = c.GetSnapshotUri
	}

	b, err := getUri(token)
	if err != nil {
		return "", err
	}

	rawURL := FindTagValue(b, "Uri")
	rawURL = strings.TrimSpace(html.UnescapeString(rawURL))

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	if u.User == nil && c.url.User != nil {
		u.User = c.url.User
	}

	return u.String(), nil
}

func (c *Client) GetName() (string, error) {
	b, err := c.DeviceRequest(DeviceGetDeviceInformation)
	if err != nil {
		return "", err
	}

	return FindTagValue(b, "Manufacturer") + " " + FindTagValue(b, "Model"), nil
}

func (c *Client) GetProfilesTokens() ([]string, error) {
	b, err := c.MediaRequest(MediaGetProfiles)
	if err != nil {
		return nil, err
	}

	var tokens []string

	re := regexp.MustCompile(`Profiles.+?token="([^"]+)`)
	for _, s := range re.FindAllStringSubmatch(string(b), 10) {
		tokens = append(tokens, s[1])
	}

	return tokens, nil
}

func (c *Client) HasSnapshots() bool {
	b, err := c.GetServiceCapabilities()
	if err != nil {
		return false
	}
	return strings.Contains(string(b), `SnapshotUri="true"`)
}

func (c *Client) GetProfile(token string) ([]byte, error) {
	return c.Request(
		c.mediaURL, `<trt:GetProfile><trt:ProfileToken>`+token+`</trt:ProfileToken></trt:GetProfile>`,
	)
}

func (c *Client) GetVideoSourceConfiguration(token string) ([]byte, error) {
	return c.Request(c.mediaURL, `<trt:GetVideoSourceConfiguration>
	<trt:ConfigurationToken>`+token+`</trt:ConfigurationToken>
</trt:GetVideoSourceConfiguration>`)
}

func (c *Client) GetStreamUri(token string) ([]byte, error) {
	return c.Request(c.mediaURL, `<trt:GetStreamUri>
	<trt:StreamSetup>
		<tt:Stream>RTP-Unicast</tt:Stream>
		<tt:Transport><tt:Protocol>RTSP</tt:Protocol></tt:Transport>
	</trt:StreamSetup>
	<trt:ProfileToken>`+token+`</trt:ProfileToken>
</trt:GetStreamUri>`)
}

func (c *Client) GetSnapshotUri(token string) ([]byte, error) {
	return c.Request(
		c.imaginURL, `<trt:GetSnapshotUri><trt:ProfileToken>`+token+`</trt:ProfileToken></trt:GetSnapshotUri>`,
	)
}

func (c *Client) GetServiceCapabilities() ([]byte, error) {
	// some cameras answer GetServiceCapabilities for media only for path = "/onvif/media"
	return c.Request(
		c.mediaURL, `<trt:GetServiceCapabilities />`,
	)
}

func (c *Client) DeviceRequest(operation string) ([]byte, error) {
	switch operation {
	case DeviceGetServices:
		operation = `<tds:GetServices><tds:IncludeCapability>true</tds:IncludeCapability></tds:GetServices>`
	case DeviceGetCapabilities:
		operation = `<tds:GetCapabilities><tds:Category>All</tds:Category></tds:GetCapabilities>`
	default:
		operation = `<tds:` + operation + `/>`
	}
	return c.Request(c.deviceURL, operation)
}

func (c *Client) MediaRequest(operation string) ([]byte, error) {
	operation = `<trt:` + operation + `/>`
	return c.Request(c.mediaURL, operation)
}

func (c *Client) Request(rawUrl, body string) ([]byte, error) {
	if rawUrl == "" {
		return nil, errors.New("onvif: unsupported service")
	}

	u, err := url.Parse(rawUrl)
	if err != nil {
		return nil, err
	}

	host := u.Host
	if u.Port() == "" {
		host += ":80"
	}

	const timeout = 5 * time.Second

	conn, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	e := NewEnvelopeWithUser(c.url.User)
	e.Append(body)
	buf := e.Bytes()

	req := &http.Request{
		Method:        "POST",
		URL:           u,
		Proto:         "HTTP/1.1",
		Header:        http.Header{"Content-Type": {"application/soap+xml;charset=utf-8"}},
		Body:          io.NopCloser(bytes.NewReader(buf)),
		ContentLength: int64(len(buf)),
		Close:         true,
	}

	_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	if err = req.Write(conn); err != nil {
		return nil, err
	}

	rd := bufio.NewReaderSize(conn, 16*1024)

	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	res, err := http.ReadResponse(rd, req)
	if err != nil {
		// Try to fix broken response https://github.com/AlexxIT/go2rtc/pull/1589
		if buf, err = io.ReadAll(rd); err != nil {
			return nil, err
		}

		// Look for XML in complete response
		if i := bytes.Index(buf, []byte("<?xml")); i > 0 {
			return buf[i:], nil
		}

		return nil, fmt.Errorf("onvif: broken response: %.100s", buf)
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.New("onvif: wrong response " + res.Status)
	}

	if buf, err = io.ReadAll(res.Body); err != nil {
		return nil, err
	}

	return buf, nil
}
