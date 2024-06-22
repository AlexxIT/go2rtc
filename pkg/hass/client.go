package hass

import (
	"errors"
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v3"
)

type Client struct {
	conn *webrtc.Conn
}

func NewClient(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()

	entityID := query.Get("entity_id")
	if entityID == "" {
		return nil, errors.New("hass: no entity_id")
	}

	var uri, token string

	if u.Host == "supervisor" {
		uri = "ws://supervisor/core/websocket"
		token = SupervisorToken()
	} else {
		uri = "ws://" + u.Host + "/api/websocket"
		token = query.Get("token")
	}

	if token == "" {
		return nil, errors.New("hass: no token")
	}

	// 1. Check connection to Hass
	hassAPI, err := NewAPI(uri, token)
	if err != nil {
		return nil, err
	}

	defer hassAPI.Close()

	// 2. Create WebRTC client
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
	conn.FormatName = "hass/webrtc"
	conn.Mode = core.ModeActiveProducer
	conn.Protocol = "ws"
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
	answer, err := hassAPI.ExchangeSDP(entityID, offer)
	if err != nil {
		return nil, err
	}

	// 5. Set answer with remote medias
	if err = conn.SetAnswer(answer); err != nil {
		return nil, err
	}

	return &Client{conn: conn}, nil
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
	return c.conn.Start()
}

func (c *Client) Stop() error {
	return c.conn.Stop()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	return c.conn.MarshalJSON()
}
