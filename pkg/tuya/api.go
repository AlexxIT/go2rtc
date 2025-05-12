package tuya

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/google/uuid"
	pionWebrtc "github.com/pion/webrtc/v4"
)

type TuyaClient struct {
	httpClient     *http.Client
	mqtt           *TuyaMQTT
	apiURL         string
	rtspURL        string
	hlsURL         string
	sessionId      string
	clientId       string
	clientSecret   string
	deviceId       string
	accessToken    string
	refreshToken   string
	expireTime     int64
	uid            string
	motoId         string
	auth           string
	skill          *Skill
	iceServers     []pionWebrtc.ICEServer
	medias         []*core.Media
	hasBackchannel bool
}

type Token struct {
	UID          string `json:"uid"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpireTime   int64  `json:"expire_time"`
}

type AudioAttributes struct {
	CallMode           []int `json:"call_mode"`           // 1 = one way, 2 = two way
	HardwareCapability []int `json:"hardware_capability"` // 1 = mic, 2 = speaker
}

type OpenApiICE struct {
	Urls       string `json:"urls"`
	Username   string `json:"username"`
	Credential string `json:"credential"`
	TTL        int    `json:"ttl"`
}

type WebICE struct {
	Urls       string `json:"urls"`
	Username   string `json:"username,omitempty"`
	Credential string `json:"credential,omitempty"`
}

type P2PConfig struct {
	Ices []OpenApiICE `json:"ices"`
}

type Skill struct {
	WebRTC int `json:"webrtc"`
	Audios []struct {
		Channels   int `json:"channels"`
		DataBit    int `json:"dataBit"`
		CodecType  int `json:"codecType"`
		SampleRate int `json:"sampleRate"`
	} `json:"audios"`
	Videos []struct {
		StreamType int    `json:"streamType"` // streamType = 2 => main stream - streamType = 4 => sub stream
		ProfileId  string `json:"profileId"`
		Width      int    `json:"width"`
		CodecType  int    `json:"codecType"`
		SampleRate int    `json:"sampleRate"`
		Height     int    `json:"height"`
	} `json:"videos"`
}

type WebRTConfig struct {
	AudioAttributes      AudioAttributes `json:"audio_attributes"`
	Auth                 string          `json:"auth"`
	ID                   string          `json:"id"`
	MotoID               string          `json:"moto_id"`
	P2PConfig            P2PConfig       `json:"p2p_config"`
	ProtocolVersion      string          `json:"protocol_version"`
	Skill                string          `json:"skill"`
	SupportsWebRTCRecord bool            `json:"supports_webrtc_record"`
	SupportsWebRTC       bool            `json:"supports_webrtc"`
	VedioClaritiy        int             `json:"vedio_clarity"`
	VideoClaritiy        int             `json:"video_clarity"`
	VideoClarities       []int           `json:"video_clarities"`
}

type OpenIoTHubConfig struct {
	Url       string `json:"url"`
	ClientID  string `json:"client_id"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	SinkTopic struct {
		IPC string `json:"ipc"`
	} `json:"sink_topic"`
	SourceSink struct {
		IPC string `json:"ipc"`
	} `json:"source_topic"`
	ExpireTime int `json:"expire_time"`
}

type WebRTCConfigResponse struct {
	Success bool        `json:"success"`
	Result  WebRTConfig `json:"result"`
	Msg     string      `json:"msg,omitempty"`
	Code    int         `json:"code,omitempty"`
}

type TokenResponse struct {
	Success bool   `json:"success"`
	Result  Token  `json:"result"`
	Msg     string `json:"msg,omitempty"`
	Code    int    `json:"code,omitempty"`
}

type AllocateRequest struct {
	Type string `json:"type"`
}

type AllocateResponse struct {
	Success bool `json:"success"`
	Result  struct {
		URL string `json:"url"`
	} `json:"result"`
	Msg  string `json:"msg,omitempty"`
	Code int    `json:"code,omitempty"`
}

type OpenIoTHubConfigRequest struct {
	UID      string `json:"uid"`
	UniqueID string `json:"unique_id"`
	LinkType string `json:"link_type"`
	Topics   string `json:"topics"`
}

type OpenIoTHubConfigResponse struct {
	Success bool             `json:"success"`
	Result  OpenIoTHubConfig `json:"result"`
	Msg     string           `json:"msg,omitempty"`
	Code    int              `json:"code,omitempty"`
}

const (
	defaultTimeout = 5 * time.Second
)

func NewTuyaClient(openAPIURL string, deviceId string, uid string, clientId string, clientSecret string, streamMode string) (*TuyaClient, error) {
	client := &TuyaClient{
		httpClient:     &http.Client{Timeout: defaultTimeout},
		mqtt:           &TuyaMQTT{waiter: core.Waiter{}},
		apiURL:         openAPIURL,
		sessionId:      core.RandString(6, 62),
		clientId:       clientId,
		deviceId:       deviceId,
		clientSecret:   clientSecret,
		uid:            uid,
		hasBackchannel: false,
	}

	if err := client.InitToken(); err != nil {
		return nil, fmt.Errorf("failed to initialize token: %w", err)
	}

	if streamMode == "rtsp" {
		if err := client.GetStreamUrl("rtsp"); err != nil {
			return nil, fmt.Errorf("failed to get RTSP URL: %w", err)
		}
	} else if streamMode == "hls" {
		if err := client.GetStreamUrl("hls"); err != nil {
			return nil, fmt.Errorf("failed to get HLS URL: %w", err)
		}
	} else {
		if err := client.InitDevice(); err != nil {
			return nil, fmt.Errorf("failed to initialize device: %w", err)
		}

		if err := client.StartMQTT(); err != nil {
			return nil, fmt.Errorf("failed to start MQTT: %w", err)
		}
	}

	return client, nil
}

func (c *TuyaClient) Close() {
	c.StopMQTT()
	c.httpClient.CloseIdleConnections()
}

func (c *TuyaClient) Request(method string, url string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	ts := time.Now().UnixNano() / 1000000
	sign := c.calBusinessSign(ts)

	req.Header.Set("Accept", "*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Access-Control-Allow-Origin", "*")
	req.Header.Set("Access-Control-Allow-Methods", "*")
	req.Header.Set("Access-Control-Allow-Headers", "*")
	req.Header.Set("mode", "no-cors")
	req.Header.Set("client_id", c.clientId)
	req.Header.Set("access_token", c.accessToken)
	req.Header.Set("sign", sign)
	req.Header.Set("t", strconv.FormatInt(ts, 10))

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	res, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		return nil, err
	}

	return res, nil
}

func (c *TuyaClient) InitToken() (err error) {
	url := fmt.Sprintf("https://%s/v1.0/token?grant_type=1", c.apiURL)

	c.accessToken = ""
	c.refreshToken = ""

	body, err := c.Request("GET", url, nil)
	if err != nil {
		return err
	}

	var tokenResponse TokenResponse
	err = json.Unmarshal(body, &tokenResponse)
	if err != nil {
		return err
	}

	if !tokenResponse.Success {
		return fmt.Errorf("error: %s", tokenResponse.Msg)
	}

	c.accessToken = tokenResponse.Result.AccessToken
	c.refreshToken = tokenResponse.Result.RefreshToken
	c.expireTime = tokenResponse.Result.ExpireTime

	return nil
}

func (c *TuyaClient) InitDevice() (err error) {
	url := fmt.Sprintf("https://%s/v1.0/users/%s/devices/%s/webrtc-configs", c.apiURL, c.uid, c.deviceId)

	body, err := c.Request("GET", url, nil)
	if err != nil {
		return err
	}

	var webRTCConfigResponse WebRTCConfigResponse
	err = json.Unmarshal(body, &webRTCConfigResponse)
	if err != nil {
		return err
	}

	if !webRTCConfigResponse.Success {
		return fmt.Errorf("error: %s", webRTCConfigResponse.Msg)
	}

	c.motoId = webRTCConfigResponse.Result.MotoID
	c.auth = webRTCConfigResponse.Result.Auth

	c.skill = &Skill{
		Audios: []struct {
			Channels   int `json:"channels"`
			DataBit    int `json:"dataBit"`
			CodecType  int `json:"codecType"`
			SampleRate int `json:"sampleRate"`
		}{},
		Videos: []struct {
			StreamType int    `json:"streamType"`
			ProfileId  string `json:"profileId"`
			Width      int    `json:"width"`
			CodecType  int    `json:"codecType"`
			SampleRate int    `json:"sampleRate"`
			Height     int    `json:"height"`
		}{},
	}

	if webRTCConfigResponse.Result.Skill != "" {
		_ = json.Unmarshal([]byte(webRTCConfigResponse.Result.Skill), c.skill)
	}

	var audioDirection string
	if contains(webRTCConfigResponse.Result.AudioAttributes.CallMode, 2) &&
		contains(webRTCConfigResponse.Result.AudioAttributes.HardwareCapability, 1) {
		audioDirection = core.DirectionSendRecv
		c.hasBackchannel = true
	} else {
		audioDirection = core.DirectionRecvonly
		c.hasBackchannel = false
	}

	c.medias = make([]*core.Media, 0)

	if len(c.skill.Audios) > 0 {
		// Use the first Audio-Codec
		audio := c.skill.Audios[0]

		c.medias = append(c.medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: audioDirection,
			Codecs: []*core.Codec{
				{
					Name:      "PCMU",
					ClockRate: uint32(audio.SampleRate),
					Channels:  uint8(audio.Channels),
				},
			},
		})
	} else {
		// Use default values for Audio
		c.medias = append(c.medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{
					Name:      "PCMU",
					ClockRate: uint32(8000),
					Channels:  uint8(1),
				},
			},
		})
	}

	if len(c.skill.Videos) > 0 {
		// Use the first Video-Codec
		video := c.skill.Videos[0]

		c.medias = append(c.medias, &core.Media{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{
					Name:        core.CodecH265,
					ClockRate:   uint32(video.SampleRate),
					PayloadType: 96,
				},
				{
					Name:        core.CodecH264,
					ClockRate:   uint32(video.SampleRate),
					PayloadType: 96,
				},
			},
		})
	} else {
		// Use default values for Video
		c.medias = append(c.medias, &core.Media{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{
					Name:        core.CodecH264,
					ClockRate:   uint32(90000),
					PayloadType: 96,
				},
			},
		})
	}

	iceServersBytes, err := json.Marshal(&webRTCConfigResponse.Result.P2PConfig.Ices)
	if err != nil {
		return err
	}

	c.iceServers, err = webrtc.UnmarshalICEServers([]byte(iceServersBytes))
	if err != nil {
		return err
	}

	return nil
}

func (c *TuyaClient) GetStreamUrl(streamType string) (err error) {
	url := fmt.Sprintf("https://%s/v1.0/devices/%s/stream/actions/allocate", c.apiURL, c.deviceId)

	request := &AllocateRequest{
		Type: streamType,
	}

	body, err := c.Request("POST", url, request)
	if err != nil {
		return err
	}

	var allosResponse AllocateResponse
	err = json.Unmarshal(body, &allosResponse)
	if err != nil {
		return err
	}

	if !allosResponse.Success {
		return fmt.Errorf("error: %s", allosResponse.Msg)
	}

	switch streamType {
	case "rtsp":
		c.rtspURL = allosResponse.Result.URL
	case "hls":
		c.hlsURL = allosResponse.Result.URL
	default:
		return fmt.Errorf("unsupported stream type: %s", streamType)
	}

	return nil
}

func (c *TuyaClient) LoadHubConfig() (config *OpenIoTHubConfig, err error) {
	url := fmt.Sprintf("https://%s/v2.0/open-iot-hub/access/config", c.apiURL)

	request := &OpenIoTHubConfigRequest{
		UID:      c.uid,
		UniqueID: uuid.New().String(),
		LinkType: "mqtt",
		Topics:   "ipc",
	}

	body, err := c.Request("POST", url, request)
	if err != nil {
		return nil, err
	}

	var openIoTHubConfigResponse OpenIoTHubConfigResponse
	err = json.Unmarshal(body, &openIoTHubConfigResponse)
	if err != nil {
		return nil, err
	}

	if !openIoTHubConfigResponse.Success {
		return nil, fmt.Errorf("error: %s", openIoTHubConfigResponse.Msg)
	}

	return &openIoTHubConfigResponse.Result, nil
}

// Search the streamType based on the selection "main" or "sub"
func (c *TuyaClient) getStreamType(streamChoice string) uint32 {
	// Default streamType if nothing is found
	defaultStreamType := uint32(1)

	if c.skill == nil || len(c.skill.Videos) == 0 {
		return defaultStreamType
	}

	// Find the highest and lowest resolution
	var highestResType uint32 = defaultStreamType
	var highestRes int = 0
	var lowestResType uint32 = defaultStreamType
	var lowestRes int = 0

	for _, video := range c.skill.Videos {
		res := video.Width * video.Height

		// Highest Resolution
		if res > highestRes {
			highestRes = res
			highestResType = uint32(video.StreamType)
		}

		// Lower Resolution (or first if not set yet)
		if lowestRes == 0 || res < lowestRes {
			lowestRes = res
			lowestResType = uint32(video.StreamType)
		}
	}

	// Return the streamType based on the selection
	switch streamChoice {
	case "main":
		return highestResType
	case "sub":
		return lowestResType
	default:
		return defaultStreamType
	}
}

func (c *TuyaClient) calBusinessSign(ts int64) string {
	data := fmt.Sprintf("%s%s%s%d", c.clientId, c.accessToken, c.clientSecret, ts)
	val := md5.Sum([]byte(data))
	res := fmt.Sprintf("%X", val)
	return res
}

func contains(slice []int, val int) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
