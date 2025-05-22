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

	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/google/uuid"
)

type Token struct {
	UID          string `json:"uid"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpireTime   int64  `json:"expire_time"`
}

type WebRTCConfigResponse struct {
	Timestamp int64        `json:"t"`
	Success   bool         `json:"success"`
	Result    WebRTCConfig `json:"result"`
	Msg       string       `json:"msg,omitempty"`
	Code      int          `json:"code,omitempty"`
}

type TokenResponse struct {
	Timestamp int64  `json:"t"`
	Success   bool   `json:"success"`
	Result    Token  `json:"result"`
	Msg       string `json:"msg,omitempty"`
	Code      int    `json:"code,omitempty"`
}

type OpenIoTHubConfigRequest struct {
	UID      string `json:"uid"`
	UniqueID string `json:"unique_id"`
	LinkType string `json:"link_type"`
	Topics   string `json:"topics"`
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

type OpenIoTHubConfigResponse struct {
	Timestamp int              `json:"t"`
	Success   bool             `json:"success"`
	Result    OpenIoTHubConfig `json:"result"`
	Msg       string           `json:"msg,omitempty"`
	Code      int              `json:"code,omitempty"`
}

type TuyaCloudApiClient struct {
	TuyaClient
	clientId        string
	clientSecret    string
	refreshingToken bool
}

func NewTuyaCloudApiClient(baseUrl string, uid string, deviceId string, clientId string, clientSecret string, streamMode string) (*TuyaCloudApiClient, error) {
	mqttClient := NewTuyaMqttClient(deviceId)

	client := &TuyaCloudApiClient{
		TuyaClient: TuyaClient{
			httpClient: &http.Client{Timeout: 15 * time.Second},
			mqtt:       mqttClient,
			uid:        uid,
			deviceId:   deviceId,
			streamMode: streamMode,
			expireTime: 0,
			baseUrl:    baseUrl,
		},
		clientId:        clientId,
		clientSecret:    clientSecret,
		refreshingToken: false,
	}

	return client, nil
}

// WebRTC Flow
func (c *TuyaCloudApiClient) Init() error {
	if err := c.initToken(); err != nil {
		return fmt.Errorf("failed to initialize token: %w", err)
	}

	webrtcConfig, err := c.loadWebrtcConfig()
	if err != nil {
		return fmt.Errorf("failed to load webrtc config: %w", err)
	}

	hubConfig, err := c.loadHubConfig()
	if err != nil {
		return fmt.Errorf("failed to load hub config: %w", err)
	}

	if err := c.mqtt.Start(hubConfig, webrtcConfig, c.skill.WebRTC); err != nil {
		return fmt.Errorf("failed to start MQTT: %w", err)
	}

	return nil
}

func (c *TuyaCloudApiClient) GetStreamUrl(streamType string) (streamUrl string, err error) {
	if err := c.initToken(); err != nil {
		return "", fmt.Errorf("failed to initialize token: %w", err)
	}

	url := fmt.Sprintf("https://%s/v1.0/devices/%s/stream/actions/allocate", c.baseUrl, c.deviceId)

	request := &AllocateRequest{
		Type: streamType,
	}

	body, err := c.request("POST", url, request)
	if err != nil {
		return "", err
	}

	var allocResponse AllocateResponse
	err = json.Unmarshal(body, &allocResponse)
	if err != nil {
		return "", err
	}

	if !allocResponse.Success {
		return "", fmt.Errorf(allocResponse.Msg)
	}

	return allocResponse.Result.URL, nil
}

func (c *TuyaCloudApiClient) initToken() (err error) {
	if c.refreshingToken {
		return nil
	}

	now := time.Now().Unix()
	if (c.expireTime - 60) > now {
		return nil
	}

	c.refreshingToken = true

	url := fmt.Sprintf("https://%s/v1.0/token?grant_type=1", c.baseUrl)

	c.accessToken = ""
	c.refreshToken = ""

	body, err := c.request("GET", url, nil)
	if err != nil {
		return err
	}

	var tokenResponse TokenResponse
	err = json.Unmarshal(body, &tokenResponse)
	if err != nil {
		return err
	}

	if !tokenResponse.Success {
		return fmt.Errorf(tokenResponse.Msg)
	}

	c.accessToken = tokenResponse.Result.AccessToken
	c.refreshToken = tokenResponse.Result.RefreshToken
	c.expireTime = tokenResponse.Timestamp + tokenResponse.Result.ExpireTime
	c.refreshingToken = false

	return nil
}

func (c *TuyaCloudApiClient) loadWebrtcConfig() (*WebRTCConfig, error) {
	url := fmt.Sprintf("https://%s/v1.0/users/%s/devices/%s/webrtc-configs", c.baseUrl, c.uid, c.deviceId)

	body, err := c.request("GET", url, nil)
	if err != nil {
		return nil, err
	}

	var webRTCConfigResponse WebRTCConfigResponse
	err = json.Unmarshal(body, &webRTCConfigResponse)
	if err != nil {
		return nil, err
	}

	if !webRTCConfigResponse.Success {
		return nil, fmt.Errorf(webRTCConfigResponse.Msg)
	}

	err = json.Unmarshal([]byte(webRTCConfigResponse.Result.Skill), &c.skill)
	if err != nil {
		return nil, err
	}

	iceServers, err := json.Marshal(&webRTCConfigResponse.Result.P2PConfig.Ices)
	if err != nil {
		return nil, err
	}

	c.iceServers, err = webrtc.UnmarshalICEServers(iceServers)
	if err != nil {
		return nil, err
	}

	return &webRTCConfigResponse.Result, nil
}

func (c *TuyaCloudApiClient) loadHubConfig() (config *MQTTConfig, err error) {
	url := fmt.Sprintf("https://%s/v2.0/open-iot-hub/access/config", c.baseUrl)

	request := &OpenIoTHubConfigRequest{
		UID:      c.uid,
		UniqueID: uuid.New().String(),
		LinkType: "mqtt",
		Topics:   "ipc",
	}

	body, err := c.request("POST", url, request)
	if err != nil {
		return nil, err
	}

	var openIoTHubConfigResponse OpenIoTHubConfigResponse
	err = json.Unmarshal(body, &openIoTHubConfigResponse)
	if err != nil {
		return nil, err
	}

	if !openIoTHubConfigResponse.Success {
		return nil, fmt.Errorf(openIoTHubConfigResponse.Msg)
	}

	return &MQTTConfig{
		Url:            openIoTHubConfigResponse.Result.Url,
		Username:       openIoTHubConfigResponse.Result.Username,
		Password:       openIoTHubConfigResponse.Result.Password,
		ClientID:       openIoTHubConfigResponse.Result.ClientID,
		PublishTopic:   openIoTHubConfigResponse.Result.SinkTopic.IPC,
		SubscribeTopic: openIoTHubConfigResponse.Result.SourceSink.IPC,
	}, nil
}

func (c *TuyaCloudApiClient) request(method string, url string, body any) ([]byte, error) {
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

func (c *TuyaCloudApiClient) calBusinessSign(ts int64) string {
	data := fmt.Sprintf("%s%s%s%d", c.clientId, c.accessToken, c.clientSecret, ts)
	val := md5.Sum([]byte(data))
	res := fmt.Sprintf("%X", val)
	return res
}
