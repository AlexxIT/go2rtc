package tuya

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	TUYA_HOST      = "apigw.iotbing.com"
	TUYA_CLIENT_ID = "HA_3y9q4ak7g4ephrvke"
	TUYA_SCHEMA    = "haauthorize"
)

type OpenApiMQTTConfig struct {
	ClientID   string `json:"clientId"`
	ExpireTime int    `json:"expireTime"`
	Password   string `json:"password"`
	Topic      struct {
		DevID struct {
			Pub string `json:"pub"`
			Sub string `json:"sub"`
		} `json:"devId"`
		OwnerID struct {
			Sub string `json:"sub"`
		} `json:"ownerId"`
	} `json:"topic"`
	URL      string `json:"url"`
	Username string `json:"username"`
}

type OpenApiMQTTConfigRequest struct {
	LinkID string `json:"linkId"`
}

type OpenApiMQTTConfigResponse struct {
	Success bool              `json:"success"`
	Result  OpenApiMQTTConfig `json:"result"`
	Msg     string            `json:"msg,omitempty"`
}

type TokenInfo struct {
	AccessToken  string `json:"access_token"`
	ExpireTime   int64  `json:"expire_time"`
	RefreshToken string `json:"refresh_token"`
}

type LoginResult struct {
	AccessToken  string `json:"access_token"`
	Endpoint     string `json:"endpoint"`
	ExpireTime   int64  `json:"expire_time"` // seconds
	RefreshToken string `json:"refresh_token"`
	TerminalID   string `json:"terminal_id"`
	UID          string `json:"uid"`
	Username     string `json:"username"`
}

type LoginResponse struct {
	Timestamp int64       `json:"t"`
	Success   bool        `json:"success"`
	Result    LoginResult `json:"result"`
	Msg       string      `json:"msg,omitempty"`
}

type QRResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Code string `json:"qrcode"`
	} `json:"result"`
	Msg string `json:"msg,omitempty"`
}

type Home struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	OwnerID     string  `json:"ownerId"`
	Background  string  `json:"background"`
	GeoName     string  `json:"geoName"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	GmtCreate   int64   `json:"gmtCreate"`
	GmtModified int64   `json:"gmtModified"`
	GroupID     int64   `json:"groupId"`
	Status      bool    `json:"status"`
	UID         string  `json:"uid"`
}

type HomesResponse struct {
	Success bool   `json:"success"`
	Result  []Home `json:"result"`
	Msg     string `json:"msg,omitempty"`
}

type DeviceFunction struct {
	Code   string         `json:"code"`
	Desc   string         `json:"desc"`
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	Values map[string]any `json:"values"`
}

type DeviceStatusRange struct {
	Code   string         `json:"code"`
	Type   string         `json:"type"`
	Values map[string]any `json:"values"`
}

type Device struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	LocalKey    string `json:"local_key"`
	Category    string `json:"category"`
	ProductID   string `json:"product_id"`
	ProductName string `json:"product_name"`
	Sub         bool   `json:"sub"`
	UUID        string `json:"uuid"`
	AssetID     string `json:"asset_id"`
	Online      bool   `json:"online"`
	Icon        string `json:"icon"`
	IP          string `json:"ip"`
	TimeZone    string `json:"time_zone"`
	ActiveTime  int64  `json:"active_time"`
	CreateTime  int64  `json:"create_time"`
	UpdateTime  int64  `json:"update_time"`
}

type DeviceRequest struct {
	HomeID string `json:"homeId"`
}

type DeviceResponse struct {
	Success bool     `json:"success"`
	Result  []Device `json:"result"`
	Msg     string   `json:"msg,omitempty"`
}

type TuyaOpenApiClient struct {
	TuyaClient
	terminalId      string
	refreshingToken bool
}

func NewTuyaOpenApiClient(
	baseUrl string,
	uid string,
	deviceId string,
	terminalId string,
	tokenInfoOrString any,
	streamMode string,
) (*TuyaOpenApiClient, error) {
	tokenInfo, err := ParseTokenInfo(tokenInfoOrString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token info: %w", err)
	}

	mqttClient := NewTuyaMqttClient(deviceId)

	client := &TuyaOpenApiClient{
		TuyaClient: TuyaClient{
			httpClient:   &http.Client{Timeout: 15 * time.Second},
			mqtt:         mqttClient,
			uid:          uid,
			deviceId:     deviceId,
			accessToken:  tokenInfo.AccessToken,
			refreshToken: tokenInfo.RefreshToken,
			expireTime:   tokenInfo.ExpireTime,
			streamMode:   streamMode,
			baseUrl:      baseUrl,
		},
		terminalId:      terminalId,
		refreshingToken: false,
	}

	return client, nil
}

// WebRTC Flow (not supported yet)
func (c *TuyaOpenApiClient) Init() error {
	if err := c.initToken(); err != nil {
		return fmt.Errorf("failed to initialize token: %w", err)
	}

	return fmt.Errorf("stream mode %s is not supported", c.streamMode)
}

func (c *TuyaOpenApiClient) GetStreamUrl(streamType string) (streamUrl string, err error) {
	if err := c.initToken(); err != nil {
		return "", fmt.Errorf("failed to initialize token: %w", err)
	}

	urlPath := fmt.Sprintf("/v1.0/m/ipc/%s/stream/actions/allocate", c.deviceId)

	request := &AllocateRequest{
		Type: streamType,
	}

	body, err := c.request("POST", urlPath, nil, request)
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

func (c *TuyaOpenApiClient) GetAllDevices() ([]Device, error) {
	homes, err := c.queryHomes()
	if err != nil {
		return nil, err
	}

	time.Sleep(2 * time.Second)
	deviceMap := make(map[string]Device)

	for i, home := range homes {
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}

		devices, err := c.queryDevicesByHome(home.OwnerID)
		if err != nil {
			return nil, err
		}

		for _, device := range devices {
			// https://github.com/home-assistant/core/blob/088cfc3576e0018ad1df373c08549092918e6530/homeassistant/components/tuya/camera.py#L19
			if device.Category == "sp" || device.Category == "dghsxj" {
				deviceMap[device.ID] = device
			}
		}
	}

	var devices []Device
	for _, device := range deviceMap {
		devices = append(devices, device)
	}

	return devices, nil
}

func (c *TuyaOpenApiClient) loadHubConfig() (config *MQTTConfig, err error) {
	request := OpenApiMQTTConfigRequest{
		LinkID: fmt.Sprintf("tuya-device-sharing-sdk-go.%s", uuid.New().String()),
	}

	body, err := c.request("POST", "/v1.0/m/life/ha/access/config", nil, request)
	if err != nil {
		return nil, err
	}

	var mqttConfigResponse OpenApiMQTTConfigResponse
	if err := json.Unmarshal(body, &mqttConfigResponse); err != nil {
		return nil, err
	}

	if !mqttConfigResponse.Success {
		return nil, fmt.Errorf("failed to get MQTT config: %s", mqttConfigResponse.Msg)
	}

	return &MQTTConfig{
		Url:            mqttConfigResponse.Result.URL,
		Username:       mqttConfigResponse.Result.Username,
		Password:       mqttConfigResponse.Result.Password,
		ClientID:       mqttConfigResponse.Result.ClientID,
		PublishTopic:   mqttConfigResponse.Result.Topic.DevID.Pub,
		SubscribeTopic: mqttConfigResponse.Result.Topic.DevID.Sub,
	}, nil
}

func (c *TuyaOpenApiClient) queryHomes() ([]Home, error) {
	body, err := c.request("GET", "/v1.0/m/life/users/homes", nil, nil)
	if err != nil {
		return nil, err
	}

	var homesResponse HomesResponse
	if err := json.Unmarshal(body, &homesResponse); err != nil {
		return nil, err
	}

	if !homesResponse.Success {
		return nil, fmt.Errorf("failed to get homes: %s", homesResponse.Msg)
	}

	return homesResponse.Result, nil
}

func (c *TuyaOpenApiClient) queryDevicesByHome(homeID string) ([]Device, error) {
	params := DeviceRequest{
		HomeID: homeID,
	}

	body, err := c.request("GET", "/v1.0/m/life/ha/home/devices", params, nil)
	if err != nil {
		return nil, err
	}

	var devicesResponse DeviceResponse
	if err := json.Unmarshal(body, &devicesResponse); err != nil {
		return nil, err
	}

	if !devicesResponse.Success {
		return nil, fmt.Errorf("failed to get devices: %s", devicesResponse.Msg)
	}

	return devicesResponse.Result, nil
}

// https://github.com/tuya/tuya-device-sharing-sdk/blob/main/tuya_sharing/customerapi.py
func (c *TuyaOpenApiClient) request(
	method string,
	path string,
	params any,
	body any,
) ([]byte, error) {
	rid := uuid.New().String()
	sid := ""

	md5Hash := md5.New()
	ridRefreshToken := rid + c.refreshToken
	md5Hash.Write([]byte(ridRefreshToken))
	hashKey := hex.EncodeToString(md5Hash.Sum(nil))
	secret := SecretGenerating(rid, sid, hashKey)

	queryEncdata := ""
	var reqURL string
	if params != nil {
		jsonData := FormToJSON(params)

		encryptedData, err := AesGCMEncrypt(jsonData, secret)
		if err != nil {
			return nil, err
		}

		queryEncdata = encryptedData
		reqURL = fmt.Sprintf("https://%s%s?encdata=%s", c.baseUrl, path, queryEncdata)
	} else {
		reqURL = fmt.Sprintf("https://%s%s", c.baseUrl, path)
	}

	bodyEncdata := ""
	var reqBody io.Reader
	if body != nil {
		jsonData := FormToJSON(body)

		encryptedData, err := AesGCMEncrypt(jsonData, secret)
		if err != nil {
			return nil, err
		}

		bodyEncdata = encryptedData
		encBody := map[string]string{"encdata": bodyEncdata}
		bodyBytes, _ := json.Marshal(encBody)
		reqBody = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequest(method, reqURL, reqBody)
	if err != nil {
		return nil, err
	}

	t := time.Now().Add(2*time.Second).UnixNano() / int64(time.Millisecond)
	headers := map[string]string{
		"X-appKey":     TUYA_CLIENT_ID,
		"X-requestId":  rid,
		"X-sid":        sid,
		"X-time":       fmt.Sprintf("%d", t),
		"Content-Type": "application/json",
	}

	if c.accessToken != "" {
		headers["X-token"] = c.accessToken
	}

	sign := RestfulSign(hashKey, queryEncdata, bodyEncdata, headers)
	headers["X-sign"] = sign

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var resultObj map[string]any
	if err := json.Unmarshal(respBody, &resultObj); err != nil {
		return nil, err
	}

	if resultStr, ok := resultObj["result"].(string); ok {
		decrypted, err := AesGCMDecrypt(resultStr, secret)
		if err != nil {
			return nil, err
		}

		var decryptedObj any
		if err := json.Unmarshal([]byte(decrypted), &decryptedObj); err == nil {
			resultObj["result"] = decryptedObj
		} else {
			resultObj["result"] = decrypted
		}

		updatedResponse, err := json.Marshal(resultObj)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal updated response: %w", err)
		}

		return updatedResponse, nil
	}

	return respBody, nil
}

func (c *TuyaOpenApiClient) initToken() error {
	if c.refreshingToken {
		return nil
	}

	now := time.Now().Unix()
	if (c.expireTime - 60) > now {
		return nil
	}

	c.refreshingToken = true

	urlPath := fmt.Sprintf("/v1.0/m/token/%s", c.refreshToken)

	body, err := c.request("GET", urlPath, nil, nil)
	if err != nil {
		return err
	}

	var loginResponse LoginResponse
	if err := json.Unmarshal(body, &loginResponse); err != nil {
		return err
	}

	if !loginResponse.Success {
		return fmt.Errorf("failed to get token: %s", loginResponse.Msg)
	}

	c.accessToken = loginResponse.Result.AccessToken
	c.refreshToken = loginResponse.Result.RefreshToken
	c.expireTime = loginResponse.Timestamp + loginResponse.Result.ExpireTime
	c.refreshingToken = false

	return nil
}
