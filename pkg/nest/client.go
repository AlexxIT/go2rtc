package nest

import (
	"errors"
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v3"
)

type Client struct {
	conn *webrtc.Conn
	api  *API
}

func Dial(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	cliendID := query.Get("client_id")
	cliendSecret := query.Get("client_secret")
	refreshToken := query.Get("refresh_token")
	projectID := query.Get("project_id")
	deviceID := query.Get("device_id")

	if cliendID == "" || cliendSecret == "" || refreshToken == "" || projectID == "" || deviceID == "" {
		return nil, errors.New("nest: wrong query")
	}

	nestAPI, err := NewAPI(cliendID, cliendSecret, refreshToken)
	if err != nil {
		return nil, err
	}

	rtcAPI, err := webrtc.NewAPI()
	if err != nil {
		return nil, err
	}

	conf := pion.Configuration{}
	pc, err := rtcAPI.NewPeerConnection(conf)
	if err != nil {
		return nil, err
	}

	conn := webrtc.NewConn(pc)
	conn.FormatName = "nest/webrtc"
	conn.Mode = core.ModeActiveProducer
	conn.Protocol = "http"
	conn.URL = rawURL

	// https://developers.google.com/nest/device-access/traits/device/camera-live-stream#generatewebrtcstream-request-fields
	medias := []*core.Media{
		{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
		{Kind: "app"}, // important for Nest
	}

	// 3. Create offer with candidates
	offer, err := conn.CreateCompleteOffer(medias)
	if err != nil {
		return nil, err
	}

	// 4. Exchange SDP via Hass
	answer, err := nestAPI.ExchangeSDP(projectID, deviceID, offer)
	if err != nil {
		return nil, err
	}

	// 5. Set answer with remote medias
	if err = conn.SetAnswer(answer); err != nil {
		return nil, err
	}

	return &Client{conn: conn, api: nestAPI}, nil
}

func (c *Client) GetMedias() []*core.Media {
	return c.conn.GetMedias()
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return c.conn.GetTrack(media, codec)
}

func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	return c.conn.AddTrack(media, codec, track)
}

func (c *Client) Start() error {
	c.api.StartExtendStreamTimer()
	return c.conn.Start()
}

func (c *Client) Stop() error {
	c.api.StopExtendStreamTimer()
	return c.conn.Stop()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	return c.conn.MarshalJSON()
}
