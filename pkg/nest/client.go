package nest

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v4"
)

type WebRTCClient struct {
	conn *webrtc.Conn
	api  *API
}

type RTSPClient struct {
	conn *rtsp.Conn
	api  *API
}

func Dial(rawURL string) (core.Producer, error) {
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

	maxRetries := 3
	retryDelay := time.Second * 30

	var nestAPI *API
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		nestAPI, err = NewAPI(cliendID, cliendSecret, refreshToken)
		if err == nil {
			break
		}
		lastErr = err
		if attempt < maxRetries-1 {
			time.Sleep(retryDelay)
			retryDelay *= 2 // exponential backoff
		}
	}

	if nestAPI == nil {
		return nil, lastErr
	}

	protocols := strings.Split(query.Get("protocols"), ",")
	if len(protocols) > 0 && protocols[0] == "RTSP" {
		return rtspConn(nestAPI, rawURL, projectID, deviceID)
	}

	// Default to WEB_RTC for backwards compataiility
	return rtcConn(nestAPI, rawURL, projectID, deviceID)
}

func (c *WebRTCClient) GetMedias() []*core.Media {
	return c.conn.GetMedias()
}

func (c *WebRTCClient) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return c.conn.GetTrack(media, codec)
}

func (c *WebRTCClient) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	return c.conn.AddTrack(media, codec, track)
}

func (c *WebRTCClient) Start() error {
	c.api.StartExtendStreamTimer()
	return c.conn.Start()
}

func (c *WebRTCClient) Stop() error {
	c.api.StopExtendStreamTimer()
	return c.conn.Stop()
}

func (c *WebRTCClient) MarshalJSON() ([]byte, error) {
	return c.conn.MarshalJSON()
}

func rtcConn(nestAPI *API, rawURL, projectID, deviceID string) (*WebRTCClient, error) {
	maxRetries := 3
	retryDelay := time.Second * 30
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
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
			lastErr = err
			if attempt < maxRetries-1 {
				time.Sleep(retryDelay)
				retryDelay *= 2
				continue
			}
			return nil, err
		}

		// 5. Set answer with remote medias
		if err = conn.SetAnswer(answer); err != nil {
			return nil, err
		}

		return &WebRTCClient{conn: conn, api: nestAPI}, nil
	}

	return nil, lastErr
}

func rtspConn(nestAPI *API, rawURL, projectID, deviceID string) (*RTSPClient, error) {
	rtspURL, err := nestAPI.GenerateRtspStream(projectID, deviceID)
	if err != nil {
		return nil, err
	}

	rtspClient := rtsp.NewClient(rtspURL)
	if err := rtspClient.Dial(); err != nil {
		return nil, err
	}
	if err := rtspClient.Describe(); err != nil {
		return nil, err
	}

	return &RTSPClient{conn: rtspClient, api: nestAPI}, nil
}

func (c *RTSPClient) GetMedias() []*core.Media {
	result := c.conn.GetMedias()
	return result
}

func (c *RTSPClient) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return c.conn.GetTrack(media, codec)
}

func (c *RTSPClient) Start() error {
	c.api.StartExtendStreamTimer()
	return c.conn.Start()
}

func (c *RTSPClient) Stop() error {
	c.api.StopRTSPStream()
	c.api.StopExtendStreamTimer()
	return c.conn.Stop()
}

func (c *RTSPClient) MarshalJSON() ([]byte, error) {
	return c.conn.MarshalJSON()
}
