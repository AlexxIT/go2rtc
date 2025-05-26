package tuya

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/webrtc"
)

type LoginTokenRequest struct {
	CountryCode string `json:"countryCode"`
	Username    string `json:"username"`
	IsUid       bool   `json:"isUid"`
}

type LoginTokenResponse struct {
	Result  LoginToken `json:"result"`
	Success bool       `json:"success"`
	Msg     string     `json:"errorMsg,omitempty"`
}

type LoginToken struct {
	Token     string `json:"token"`
	Exponent  string `json:"exponent"`
	PublicKey string `json:"publicKey"`
	PbKey     string `json:"pbKey"`
}

type PasswordLoginRequest struct {
	CountryCode string `json:"countryCode"`
	Email       string `json:"email,omitempty"`
	Mobile      string `json:"mobile,omitempty"`
	Passwd      string `json:"passwd"`
	Token       string `json:"token"`
	IfEncrypt   int    `json:"ifencrypt"`
	Options     string `json:"options"`
}

type PasswordLoginResponse struct {
	Result   LoginResult `json:"result"`
	Success  bool        `json:"success"`
	Status   string      `json:"status"`
	ErrorMsg string      `json:"errorMsg,omitempty"`
}

type LoginResult struct {
	Attribute          int    `json:"attribute"`
	ClientId           string `json:"clientId"`
	DataVersion        int    `json:"dataVersion"`
	Domain             Domain `json:"domain"`
	Ecode              string `json:"ecode"`
	Email              string `json:"email"`
	Extras             Extras `json:"extras"`
	HeadPic            string `json:"headPic"`
	ImproveCompanyInfo bool   `json:"improveCompanyInfo"`
	Nickname           string `json:"nickname"`
	PartnerIdentity    string `json:"partnerIdentity"`
	PhoneCode          string `json:"phoneCode"`
	Receiver           string `json:"receiver"`
	RegFrom            int    `json:"regFrom"`
	Sid                string `json:"sid"`
	SnsNickname        string `json:"snsNickname"`
	TempUnit           int    `json:"tempUnit"`
	Timezone           string `json:"timezone"`
	TimezoneId         string `json:"timezoneId"`
	Uid                string `json:"uid"`
	UserType           int    `json:"userType"`
	Username           string `json:"username"`
}

type Domain struct {
	AispeechHttpsUrl    string `json:"aispeechHttpsUrl"`
	AispeechQuicUrl     string `json:"aispeechQuicUrl"`
	DeviceHttpUrl       string `json:"deviceHttpUrl"`
	DeviceHttpsPskUrl   string `json:"deviceHttpsPskUrl"`
	DeviceHttpsUrl      string `json:"deviceHttpsUrl"`
	DeviceMediaMqttUrl  string `json:"deviceMediaMqttUrl"`
	DeviceMediaMqttsUrl string `json:"deviceMediaMqttsUrl"`
	DeviceMqttsPskUrl   string `json:"deviceMqttsPskUrl"`
	DeviceMqttsUrl      string `json:"deviceMqttsUrl"`
	GwApiUrl            string `json:"gwApiUrl"`
	GwMqttUrl           string `json:"gwMqttUrl"`
	HttpPort            int    `json:"httpPort"`
	HttpsPort           int    `json:"httpsPort"`
	HttpsPskPort        int    `json:"httpsPskPort"`
	MobileApiUrl        string `json:"mobileApiUrl"`
	MobileMediaMqttUrl  string `json:"mobileMediaMqttUrl"`
	MobileMqttUrl       string `json:"mobileMqttUrl"`
	MobileMqttsUrl      string `json:"mobileMqttsUrl"`
	MobileQuicUrl       string `json:"mobileQuicUrl"`
	MqttPort            int    `json:"mqttPort"`
	MqttQuicUrl         string `json:"mqttQuicUrl"`
	MqttsPort           int    `json:"mqttsPort"`
	MqttsPskPort        int    `json:"mqttsPskPort"`
	RegionCode          string `json:"regionCode"`
}

type Extras struct {
	HomeId    string `json:"homeId"`
	SceneType string `json:"sceneType"`
}

type AppInfoResponse struct {
	Result  AppInfo `json:"result"`
	T       int64   `json:"t"`
	Success bool    `json:"success"`
	Msg     string  `json:"errorMsg,omitempty"`
}

type AppInfo struct {
	AppId    int    `json:"appId"`
	AppName  string `json:"appName"`
	ClientId string `json:"clientId"`
	Icon     string `json:"icon"`
}

type MQTTConfigResponse struct {
	Result  TuyaApiMQTTConfig `json:"result"`
	Success bool              `json:"success"`
	Msg     string            `json:"errorMsg,omitempty"`
}

type TuyaApiMQTTConfig struct {
	Msid     string `json:"msid"`
	Password string `json:"password"`
}

type HomeListResponse struct {
	Result  []Home `json:"result"`
	T       int64  `json:"t"`
	Success bool   `json:"success"`
	Msg     string `json:"errorMsg,omitempty"`
}

type SharedHomeListResponse struct {
	Result  SharedHome `json:"result"`
	T       int64      `json:"t"`
	Success bool       `json:"success"`
	Msg     string     `json:"errorMsg,omitempty"`
}

type SharedHome struct {
	SecurityWebCShareInfoList []struct {
		DeviceInfoList []Device `json:"deviceInfoList"`
		Nickname       string   `json:"nickname"`
		Username       string   `json:"username"`
	} `json:"securityWebCShareInfoList"`
}

type Home struct {
	Admin            bool    `json:"admin"`
	Background       string  `json:"background"`
	DealStatus       int     `json:"dealStatus"`
	DisplayOrder     int     `json:"displayOrder"`
	GeoName          string  `json:"geoName"`
	Gid              int     `json:"gid"`
	GmtCreate        int64   `json:"gmtCreate"`
	GmtModified      int64   `json:"gmtModified"`
	GroupId          int     `json:"groupId"`
	GroupUserId      int     `json:"groupUserId"`
	Id               int     `json:"id"`
	Lat              float64 `json:"lat"`
	Lon              float64 `json:"lon"`
	ManagementStatus bool    `json:"managementStatus"`
	Name             string  `json:"name"`
	OwnerId          string  `json:"ownerId"`
	Role             int     `json:"role"`
	Status           bool    `json:"status"`
	Uid              string  `json:"uid"`
}

type RoomListRequest struct {
	HomeId string `json:"homeId"`
}

type RoomListResponse struct {
	Result  []Room `json:"result"`
	T       int64  `json:"t"`
	Success bool   `json:"success"`
	Msg     string `json:"errorMsg,omitempty"`
}

type Room struct {
	DeviceCount int      `json:"deviceCount"`
	DeviceList  []Device `json:"deviceList"`
	RoomId      string   `json:"roomId"`
	RoomName    string   `json:"roomName"`
}

type Device struct {
	Category            string `json:"category"`
	DeviceId            string `json:"deviceId"`
	DeviceName          string `json:"deviceName"`
	P2pType             int    `json:"p2pType"`
	ProductId           string `json:"productId"`
	SupportCloudStorage bool   `json:"supportCloudStorage"`
	Uuid                string `json:"uuid"`
}

type TuyaApiWebRTCConfigRequest struct {
	DevId         string `json:"devId"`
	ClientTraceId string `json:"clientTraceId"`
}

type TuyaApiWebRTCConfigResponse struct {
	Result  TuyaWebRTCConfig `json:"result"`
	Success bool             `json:"success"`
	Msg     string           `json:"errorMsg,omitempty"`
}

type TuyaWebRTCConfig struct {
	AudioAttributes     AudioAttributes `json:"audioAttributes"`
	Auth                string          `json:"auth"`
	GatewayId           string          `json:"gatewayId"`
	Id                  string          `json:"id"`
	LocalKey            string          `json:"localKey"`
	MotoId              string          `json:"motoId"`
	NodeId              string          `json:"nodeId"`
	P2PConfig           P2PConfig       `json:"p2pConfig"`
	ProtocolVersion     string          `json:"protocolVersion"`
	Skill               string          `json:"skill"`
	Sub                 bool            `json:"sub"`
	SupportWebrtcRecord bool            `json:"supportWebrtcRecord"`
	SupportsPtz         bool            `json:"supportsPtz"`
	SupportsWebrtc      bool            `json:"supportsWebrtc"`
	VedioClarity        int             `json:"vedioClarity"`
	VedioClaritys       []int           `json:"vedioClaritys"`
	VideoClarity        int             `json:"videoClarity"`
}

type TuyaApiClient struct {
	TuyaClient

	email       string
	password    string
	countryCode string
	mqttsUrl    string
}

type Region struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Description string `json:"description"`
	Continent   string `json:"continent"`
}

var AvailableRegions = []Region{
	{"eu-central", "protect-eu.ismartlife.me", "Central Europe", "EU"},
	{"eu-east", "protect-we.ismartlife.me", "East Europe", "EU"},
	{"us-west", "protect-us.ismartlife.me", "West America", "AZ"},
	{"us-east", "protect-ue.ismartlife.me", "East America", "AZ"},
	{"china", "protect.ismartlife.me", "China", "AY"},
	{"india", "protect-in.ismartlife.me", "India", "IN"},
}

func NewTuyaApiClient(httpClient *http.Client, baseUrl, email, password, deviceId string) (*TuyaApiClient, error) {
	var region *Region
	for _, r := range AvailableRegions {
		if r.Host == baseUrl {
			region = &r
			break
		}
	}

	if region == nil {
		return nil, fmt.Errorf("invalid region: %s", baseUrl)
	}

	if httpClient == nil {
		httpClient = CreateHTTPClientWithSession()
	}

	mqttClient := NewTuyaMqttClient(deviceId)

	client := &TuyaApiClient{
		TuyaClient: TuyaClient{
			httpClient: httpClient,
			mqtt:       mqttClient,
			deviceId:   deviceId,
			expireTime: 0,
			baseUrl:    baseUrl,
		},
		email:       email,
		password:    password,
		countryCode: region.Continent,
	}

	return client, nil
}

// WebRTC Flow
func (c *TuyaApiClient) Init() error {
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

func (c *TuyaApiClient) GetStreamUrl(streamType string) (streamUrl string, err error) {
	return "", errors.New("not supported")
}

func (c *TuyaApiClient) GetAppInfo() (*AppInfoResponse, error) {
	url := fmt.Sprintf("https://%s/api/customized/web/app/info", c.baseUrl)

	body, err := c.request("POST", url, nil)
	if err != nil {
		return nil, err
	}

	var appInfoResponse AppInfoResponse
	if err := json.Unmarshal(body, &appInfoResponse); err != nil {
		return nil, err
	}

	if !appInfoResponse.Success {
		return nil, errors.New(appInfoResponse.Msg)
	}

	return &appInfoResponse, nil
}

func (c *TuyaApiClient) GetHomeList() (*HomeListResponse, error) {
	url := fmt.Sprintf("https://%s/api/new/common/homeList", c.baseUrl)

	body, err := c.request("POST", url, nil)
	if err != nil {
		return nil, err
	}

	var homeListResponse HomeListResponse
	if err := json.Unmarshal(body, &homeListResponse); err != nil {
		return nil, err
	}

	if !homeListResponse.Success {
		return nil, errors.New(homeListResponse.Msg)
	}

	return &homeListResponse, nil
}

func (c *TuyaApiClient) GetSharedHomeList() (*SharedHomeListResponse, error) {
	url := fmt.Sprintf("https://%s/api/new/playback/shareList", c.baseUrl)

	body, err := c.request("POST", url, nil)
	if err != nil {
		return nil, err
	}

	var sharedHomeListResponse SharedHomeListResponse
	if err := json.Unmarshal(body, &sharedHomeListResponse); err != nil {
		return nil, err
	}

	if !sharedHomeListResponse.Success {
		return nil, errors.New(sharedHomeListResponse.Msg)
	}

	return &sharedHomeListResponse, nil
}

func (c *TuyaApiClient) GetRoomList(homeId string) (*RoomListResponse, error) {
	url := fmt.Sprintf("https://%s/api/new/common/roomList", c.baseUrl)

	data := RoomListRequest{
		HomeId: homeId,
	}

	body, err := c.request("POST", url, data)
	if err != nil {
		return nil, err
	}

	var roomListResponse RoomListResponse
	if err := json.Unmarshal(body, &roomListResponse); err != nil {
		return nil, err
	}

	if !roomListResponse.Success {
		return nil, errors.New(roomListResponse.Msg)
	}

	return &roomListResponse, nil
}

func (c *TuyaApiClient) initToken() error {
	tokenUrl := fmt.Sprintf("https://%s/api/login/token", c.baseUrl)

	tokenReq := LoginTokenRequest{
		CountryCode: c.countryCode,
		Username:    c.email,
		IsUid:       false,
	}

	body, err := c.request("POST", tokenUrl, tokenReq)
	if err != nil {
		return err
	}

	var tokenResp LoginTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return err
	}

	if !tokenResp.Success {
		return errors.New(tokenResp.Msg)
	}

	encryptedPassword, err := EncryptPassword(c.password, tokenResp.Result.PbKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt password: %v", err)
	}
	var loginUrl string

	loginReq := PasswordLoginRequest{
		CountryCode: c.countryCode,
		Passwd:      encryptedPassword,
		Token:       tokenResp.Result.Token,
		IfEncrypt:   1,
		Options:     `{"group":1}`,
	}

	if IsEmailAddress(c.email) {
		loginUrl = fmt.Sprintf("https://%s/api/private/email/login", c.baseUrl)
		loginReq.Email = c.email
	} else {
		loginUrl = fmt.Sprintf("https://%s/api/private/phone/login", c.baseUrl)
		loginReq.Mobile = c.email
	}

	body, err = c.request("POST", loginUrl, loginReq)
	if err != nil {
		return err
	}

	var loginResp *PasswordLoginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return err
	}

	if !loginResp.Success {
		return errors.New(loginResp.ErrorMsg)
	}

	c.mqttsUrl = fmt.Sprintf("wss://%s/mqtt", loginResp.Result.Domain.MobileMqttsUrl)
	c.expireTime = time.Now().Unix() + 2*24*60*60 // 2 days in seconds

	return nil
}

func (c *TuyaApiClient) loadWebrtcConfig() (*WebRTCConfig, error) {
	url := fmt.Sprintf("https://%s/api/jarvis/config", c.baseUrl)

	data := TuyaApiWebRTCConfigRequest{
		DevId:         c.deviceId,
		ClientTraceId: fmt.Sprintf("%x", rand.Int63()),
	}

	body, err := c.request("POST", url, data)
	if err != nil {
		return nil, err
	}

	var webRTCConfigResponse TuyaApiWebRTCConfigResponse
	err = json.Unmarshal(body, &webRTCConfigResponse)
	if err != nil {
		return nil, err
	}

	if !webRTCConfigResponse.Success {
		return nil, errors.New(webRTCConfigResponse.Msg)
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

	return &WebRTCConfig{
		AudioAttributes:      webRTCConfigResponse.Result.AudioAttributes,
		Auth:                 webRTCConfigResponse.Result.Auth,
		ID:                   webRTCConfigResponse.Result.Id,
		MotoID:               webRTCConfigResponse.Result.MotoId,
		P2PConfig:            webRTCConfigResponse.Result.P2PConfig,
		ProtocolVersion:      webRTCConfigResponse.Result.ProtocolVersion,
		Skill:                webRTCConfigResponse.Result.Skill,
		SupportsWebRTCRecord: webRTCConfigResponse.Result.SupportWebrtcRecord,
		SupportsWebRTC:       webRTCConfigResponse.Result.SupportsWebrtc,
		VedioClaritiy:        webRTCConfigResponse.Result.VedioClarity,
		VideoClaritiy:        webRTCConfigResponse.Result.VideoClarity,
		VideoClarities:       webRTCConfigResponse.Result.VedioClaritys,
	}, nil
}

func (c *TuyaApiClient) loadHubConfig() (config *MQTTConfig, err error) {
	mqttUrl := fmt.Sprintf("https://%s/api/jarvis/mqtt", c.baseUrl)

	mqttBody, err := c.request("POST", mqttUrl, nil)
	if err != nil {
		return nil, err
	}

	var mqttConfigResponse MQTTConfigResponse
	err = json.Unmarshal(mqttBody, &mqttConfigResponse)
	if err != nil {
		return nil, err
	}

	if !mqttConfigResponse.Success {
		return nil, errors.New(mqttConfigResponse.Msg)
	}

	return &MQTTConfig{
		Url:            c.mqttsUrl,
		ClientID:       fmt.Sprintf("web_%s", mqttConfigResponse.Result.Msid),
		Username:       fmt.Sprintf("web_%s", mqttConfigResponse.Result.Msid),
		Password:       mqttConfigResponse.Result.Password,
		PublishTopic:   "/av/moto/moto_id/u/{device_id}",
		SubscribeTopic: fmt.Sprintf("/av/u/%s", mqttConfigResponse.Result.Msid),
	}, nil
}

func (c *TuyaApiClient) request(method string, url string, body any) ([]byte, error) {
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

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", fmt.Sprintf("https://%s", c.baseUrl))

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