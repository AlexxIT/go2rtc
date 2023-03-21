package isapi

import (
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"io"
	"net"
	"net/http"
	"net/url"
)

type Client struct {
	core.Listener

	url     string
	channel string
	conn    net.Conn

	medias []*core.Media
	sender *core.Sender
	send   int
}

func NewClient(rawURL string) (*Client, error) {
	// check if url is valid url
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	u.Scheme = "http"
	u.Path = ""

	return &Client{url: u.String()}, nil
}

func (c *Client) Dial() (err error) {
	link := c.url + "/ISAPI/System/TwoWayAudio/channels"
	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		return err
	}

	res, err := tcp.Do(req)
	if err != nil {
		return
	}

	if res.StatusCode != http.StatusOK {
		tcp.Close(res)
		return errors.New(res.Status)
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	xml := string(b)

	codec := core.Between(xml, `<audioCompressionType>`, `<`)
	switch codec {
	case "G.711ulaw":
		codec = core.CodecPCMU
	case "G.711alaw":
		codec = core.CodecPCMA
	default:
		return nil
	}

	c.channel = core.Between(xml, `<id>`, `<`)

	media := &core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionSendonly,
		Codecs: []*core.Codec{
			{Name: codec, ClockRate: 8000},
		},
	}
	c.medias = append(c.medias, media)

	return nil
}

func (c *Client) Open() (err error) {
	link := c.url + "/ISAPI/System/TwoWayAudio/channels/" + c.channel

	req, err := http.NewRequest("PUT", link+"/open", nil)
	if err != nil {
		return err
	}

	res, err := tcp.Do(req)
	if err != nil {
		return
	}

	tcp.Close(res)

	ctx, pconn := tcp.WithConn()
	req, err = http.NewRequestWithContext(ctx, "PUT", link+"/audioData", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", "0")

	res, err = tcp.Do(req)
	if err != nil {
		return err
	}

	c.conn = *pconn

	// just block until c.conn closed
	b := make([]byte, 1)
	_, _ = c.conn.Read(b)

	tcp.Close(res)

	return nil
}

func (c *Client) Close() (err error) {
	link := c.url + "/ISAPI/System/TwoWayAudio/channels/" + c.channel + "/close"
	req, err := http.NewRequest("PUT", link+"/open", nil)
	if err != nil {
		return err
	}

	res, err := tcp.Do(req)
	if err != nil {
		return err
	}

	tcp.Close(res)

	return nil
}

//type XMLChannels struct {
//	Channels []Channel `xml:"TwoWayAudioChannel"`
//}

//type Channel struct {
//	ID      string `xml:"id"`
//	Enabled string `xml:"enabled"`
//	Codec   string `xml:"audioCompressionType"`
//}
