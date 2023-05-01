package onvif

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/antchfx/xmlquery"

	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

type Client struct {
	url            *url.URL
	media_service  string
	image_service  string
	device_service string
}

func NewClient(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	client := Client{url: u, device_service: "onvif/device_service"}
	return &client, nil
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
			return "", errors.New("wrong subtype")
		}
		token = tokens[i]
	}

	getUri := c.GetStreamUri
	if query.Has("snapshot") {
		getUri = c.GetSnapshotUri
	}

	uri, err := getUri(token)
	if err != nil {
		return "", err
	}

	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	if u.User == nil && c.url.User != nil {
		u.User = c.url.User
	}

	return u.String(), nil
}

func (c *Client) GetName() (string, error) {
	b, err := c.GetDeviceInformation()
	if err != nil {
		return "", err
	}

	return FindTagValue(b, "Manufacturer") + " " + FindTagValue(b, "Model"), nil
}

func (c *Client) GetProfilesTokens() ([]string, error) {
	b, err := c.GetProfiles()
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

func (c *Client) GetCapabilities() ([]byte, error) {
	response, err := c.Request(
		`<GetCapabilities xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
	<Category>All</Category>
</GetCapabilities>`, c.device_service,
	)
	doc, err := xmlquery.Parse(bytes.NewReader(response))
	if err != nil {
		return nil, err
	}

	c.media_service = xmlquery.FindOne(doc, "//tt:Media/tt:XAddr").InnerText()
	c.image_service = xmlquery.FindOne(doc, "//tt:Imaging/tt:XAddr").InnerText()
	return response, err
}

func (c *Client) GetNetworkInterfaces() ([]byte, error) {
	return c.Request(`<tds:GetNetworkInterfaces xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>`, c.device_service)
}

func (c *Client) GetDeviceInformation() ([]byte, error) {
	return c.Request(`<tds:GetDeviceInformation xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>`, c.device_service)
}

func (c *Client) GetProfiles() ([]byte, error) {
	return c.Request(`<GetProfiles xmlns:trt="http://www.onvif.org/ver10/media/wsdl"/>`, c.media_service)
}

func (c *Client) GetStreamUri(token string) (string, error) {
	fmt.Println("Query Stream URI with token " + token)
	response, err := c.Request(
		`<trt:GetStreamUri xmlns:trt="http://www.onvif.org/ver10/media/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">
	<trt:StreamSetup>
		<tt:Stream>RTP-Unicast</tt:Stream>
		<tt:Transport><tt:Protocol>RTSP</tt:Protocol></tt:Transport>
	</trt:StreamSetup>
	<trt:ProfileToken>`+token+`</trt:ProfileToken>
</trt:GetStreamUri>`, c.media_service)
	if err != nil {
		return "", err
	}
	doc, err := xmlquery.Parse(bytes.NewReader(response))
	if err != nil {
		return "", err
	}

	stream_url, err := xmlquery.Query(doc, "//trt:MediaUri/tt:Uri")
	if err != nil {
		fmt.Printf("Error: %v", err)
		return "", err
	}
	fmt.Println("Stream URI: " + stream_url.InnerText())
	return stream_url.InnerText(), err
}

func (c *Client) GetSnapshotUri(token string) (string, error) {
	fmt.Println("Query Snaphot URI with token " + token)
	response, err := c.Request(
		`<GetSnapshotUri xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
	<ProfileToken>`+token+`</ProfileToken>
</GetSnapshotUri>`, c.media_service)
	if err != nil {
		return "", err
	}
	doc, err := xmlquery.Parse(bytes.NewReader(response))
	if err != nil {
		return "", err
	}

	stream_url, err := xmlquery.Query(doc, "//trt:MediaUri/tt:Uri")
	if err != nil {
		fmt.Printf("Error: %v", err)
		return "", err
	}
	fmt.Println("Snaphot URI: " + stream_url.InnerText())
	return stream_url.InnerText(), err
}

func (c *Client) GetSystemDateAndTime() ([]byte, error) {
	return c.Request(
		`<ns0:GetSystemDateAndTime xmlns:ns0="http://www.onvif.org/ver10/device/wsdl"/>`, c.device_service,
	)
}

func (c *Client) GetServiceCapabilities() ([]byte, error) {
	return c.Request(
		`<ns0:GetServiceCapabilities xmlns:ns0="http://www.onvif.org/ver10/media/wsdl"/>`, c.device_service,
	)
}

func (c *Client) SystemReboot() ([]byte, error) {
	return c.Request(
		`<tds:SystemReboot xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>`, c.device_service,
	)
}

func (c *Client) GetServices() ([]byte, error) {
	return c.Request(`<tds:GetServices xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
	<tds:IncludeCapability>true</tds:IncludeCapability>
</tds:GetServices>`, c.device_service)
}

func (c *Client) GetScopes() ([]byte, error) {
	return c.Request(`<tds:GetScopes xmlns:tds="http://www.onvif.org/ver10/device/wsdl" />`, c.device_service)
}

func (c *Client) Request(body string, path string) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteString(
		`<?xml version="1.0" encoding="UTF-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">`,
	)

	if user := c.url.User; user != nil {
		nonce := core.RandString(16, 36)
		created := time.Now().UTC().Format(time.RFC3339Nano)
		pass, _ := user.Password()

		h := sha1.New()
		h.Write([]byte(nonce + created + pass))

		buf.WriteString(`<s:Header>
<wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
<wsse:UsernameToken>
<wsse:Username>` + user.Username() + `</wsse:Username>
<wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">` + base64.StdEncoding.EncodeToString(h.Sum(nil)) + `</wsse:Password>
<wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">` + base64.StdEncoding.EncodeToString([]byte(nonce)) + `</wsse:Nonce>
<wsu:Created xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">` + created + `</wsu:Created>
</wsse:UsernameToken>
</wsse:Security>
</s:Header>`)
	}

	buf.WriteString(`<s:Body>` + body + `</s:Body></s:Envelope>`)

	client := &http.Client{Timeout: time.Second * 5000}
	res, err := client.Post(
		"http://"+c.url.Host+"/"+path,
		`application/soap+xml;charset=utf-8`,
		buf,
	)
	if err != nil {
		return nil, err
	}

	// need to close body with eny response status
	b, err := io.ReadAll(res.Body)

	if err == nil && res.StatusCode != http.StatusOK {
		err = errors.New(res.Status)
	}

	return b, err
}
