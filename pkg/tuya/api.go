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
	httpClient     	*http.Client
	mqtt 			*TuyaMQTT
	apiURL 			string
	rtspURL 		string
	hlsURL 			string
	sessionID 		string
	clientID 		string
	deviceID 		string
	accessToken 	string
	refreshToken 	string
	secret 			string
	expireTime 		int64
	uid 			string
	motoID 			string
	auth   			string
	iceServers 		[]pionWebrtc.ICEServer
	medias			[]*core.Media
}

type Token struct {
	UID		 		string `json:"uid"`
	AccessToken 	string `json:"access_token"`
	RefreshToken 	string `json:"refresh_token"`
	ExpireTime 		int64 `json:"expire_time"`
}

type AllocateRequest struct {
	Type 			string `json:"type"`
}

type AllocateResponse struct {
	Success 		bool `json:"success"`
	Result  struct {
		URL string `json:"url"`
	} `json:"result"`
}

type AudioAttributes struct {
    CallMode          []int `json:"call_mode"` // 1 = one way, 2 = two way
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
	Channels      	int 	`json:"channels"`
	DataBit      	int 	`json:"dataBit"`
	CodecType    	int 	`json:"codecType"`
	SampleRate   	int 	`json:"sampleRate"`
	} `json:"audios"`
	Videos []struct {
	StreamType   	int 	`json:"streamType"` // streamType = 2 => H265 and streamType = 4 => H264
	ProfileId    	string 	`json:"profileId"`
	Width        	int 	`json:"width"`
	CodecType    	int 	`json:"codecType"`
	SampleRate   	int 	`json:"sampleRate"`
	Height       	int 	`json:"height"`
	} `json:"videos"`
}

type WebRTConfig struct {
    AudioAttributes AudioAttributes `json:"audio_attributes"`
    Auth            string          `json:"auth"`
    ID              string          `json:"id"`
    MotoID          string          `json:"moto_id"`
    P2PConfig       P2PConfig       `json:"p2p_config"`
    Skill           string          `json:"skill"`
    SupportsWebRTC  bool            `json:"supports_webrtc"`
    VideoClaritiy   int             `json:"video_clarity"`
}

type WebRTCConfigResponse struct {
	Result WebRTConfig `json:"result"`
}

type TokenResponse struct {
    Result Token `json:"result"`
}

type OpenIoTHubConfigRequest struct {
	UID      string `json:"uid"`       
	UniqueID string `json:"unique_id"` 
	LinkType string `json:"link_type"` 
	Topics   string `json:"topics"`    
}

type OpenIoTHubConfigResponse struct {
	Success bool   				`json:"success"`
	Result  OpenIoTHubConfig 	`json:"result"`
}

type OpenIoTHubConfig struct {
	Url      string `json:"url"`       
	ClientID string `json:"client_id"` 
	Username string `json:"username"`  
	Password string `json:"password"`  

	SinkTopic struct {
		IPC string `json:"ipc"`
	} `json:"sink_topic"`

	SourceSink struct {
		IPC string `json:"ipc"`
	} `json:"source_topic"`

	ExpireTime int `json:"expire_time"` 
}

const (
	defaultTimeout     = 5 * time.Second
)

func NewTuyaClient(openAPIURL string, deviceID string, uid string, clientID string, secret string, streamType string) (*TuyaClient, error) {
	client := &TuyaClient{
		httpClient:     &http.Client{Timeout: defaultTimeout},
		mqtt:           &TuyaMQTT{waiter: core.Waiter{}},
		apiURL:         openAPIURL,
		sessionID: 	    core.RandString(6, 62),
		clientID:       clientID,
		deviceID:       deviceID,
		secret:         secret,
		uid:            uid,
	}

	if err := client.InitToken(); err != nil {
		return nil, fmt.Errorf("failed to initialize token: %w", err)
	}

	if streamType == "rtsp" {
		if err := client.GetStreamUrl("rtsp"); err != nil {
			return nil, fmt.Errorf("failed to get RTSP URL: %w", err)
		}
	} else if streamType == "hls" {
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

func(c *TuyaClient) Request(method string, url string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	ts := time.Now().UnixNano() / 1000000
	sign := c.calBusinessSign(ts)

	req.Header.Set("Accept", "*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Access-Control-Allow-Origin", "*")
	req.Header.Set("Access-Control-Allow-Methods", "*")
	req.Header.Set("Access-Control-Allow-Headers", "*")
	req.Header.Set("mode", "no-cors")
	req.Header.Set("client_id", c.clientID)
	req.Header.Set("access_token", c.accessToken)
	req.Header.Set("sign", sign)
	req.Header.Set("t", strconv.FormatInt(ts, 10))

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer response.Body.Close()

	res, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status code %d: %s", response.StatusCode, string(res))
	}

	return res, nil
}

func(c *TuyaClient) InitToken() (err error) {
	url := fmt.Sprintf("https://%s/v1.0/token?grant_type=1", c.apiURL)

	c.accessToken = ""
	c.refreshToken = ""

	body, err := c.Request("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}
	
	var tokenResponse TokenResponse
	err = json.Unmarshal(body, &tokenResponse)
	if err != nil {
		return fmt.Errorf("failed to unmarshal token response: %w", err)
	}

	c.accessToken = tokenResponse.Result.AccessToken
	c.refreshToken = tokenResponse.Result.RefreshToken
	c.expireTime = tokenResponse.Result.ExpireTime

	return nil
}

func(c *TuyaClient) InitDevice() (err error) {
	url := fmt.Sprintf("https://%s/v1.0/users/%s/devices/%s/webrtc-configs", c.apiURL, c.uid, c.deviceID)

	body, err := c.Request("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get webrtc-configs: %w", err)
	}

	var webRTCConfigResponse WebRTCConfigResponse
	err = json.Unmarshal(body, &webRTCConfigResponse)
	if err != nil {
		return fmt.Errorf("failed to unmarshal webrtc-configs response: %w", err)
	}

	c.motoID = webRTCConfigResponse.Result.MotoID
	c.auth = webRTCConfigResponse.Result.Auth

	var skill Skill
    err = json.Unmarshal([]byte(webRTCConfigResponse.Result.Skill), &skill)
    if err != nil {
        return fmt.Errorf("failed to unmarshal skill: %w", err)
    }

	var audioDirection string
	if contains(webRTCConfigResponse.Result.AudioAttributes.CallMode, 2) && contains(webRTCConfigResponse.Result.AudioAttributes.HardwareCapability, 1) {
		audioDirection = core.DirectionSendRecv
	} else {
		audioDirection = core.DirectionRecvonly
	}
	
	c.medias = make([]*core.Media, 0)
	if len(skill.Audios) > 0 {
		for _, audio := range skill.Audios {
			c.medias = append(c.medias, &core.Media{
				Kind:      core.KindAudio,
				Direction: audioDirection,
				Codecs: []*core.Codec{
					{
						Name: "PCMU",
						ClockRate: uint32(audio.SampleRate),
						Channels: uint8(audio.Channels),
					},
				},
			})
		}
	} else {
		c.medias = append(c.medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{
					Name: "PCMU",
					ClockRate: uint32(8000),
					Channels: uint8(1),
				},
			},
		})
	}

	if len(skill.Videos) > 0 {
		// take only the first video codec
		video := skill.Videos[0]
		
		var name string
		switch video.CodecType {
		case 4:
			name = core.CodecH265
		case 2:
			name = core.CodecH264
		default:
			name = core.CodecH264
		}
		
		c.medias = append(c.medias, &core.Media{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{
					Name: name,
					ClockRate: uint32(video.SampleRate),
					PayloadType: 96,
				},
			},
		})
	} else {
		c.medias = append(c.medias, &core.Media{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{
					Name: core.CodecH264,
					ClockRate: uint32(90000),
					PayloadType: 96,
				},
			},
		})
	}

	iceServersBytes, err := json.Marshal(&webRTCConfigResponse.Result.P2PConfig.Ices)
	if err != nil {
		return fmt.Errorf("failed to marshal ICE servers: %w", err)
	}


	c.iceServers, err = webrtc.UnmarshalICEServers([]byte(iceServersBytes))
	if err != nil {
		return fmt.Errorf("failed to unmarshal ICE servers: %w", err)
	}

	return nil
}

func(c *TuyaClient) GetStreamUrl(streamType string) (err error) {
	url := fmt.Sprintf("https://%s/v1.0/devices/%s/stream/actions/allocate", c.apiURL, c.deviceID)

	request := &AllocateRequest{
		Type: streamType,
	}

	body, err := c.Request("POST", url, request)
	if err != nil {
		return fmt.Errorf("failed to get rtsp url: %w", err)
	}

	var allosResponse AllocateResponse
	err = json.Unmarshal(body, &allosResponse)
	if err != nil {
		return fmt.Errorf("failed to unmarshal stream response: %w", err)
	}

	if !allosResponse.Success {
		return fmt.Errorf("failed to get stream url: %s", string(body))
	}

	switch streamType {
	case "rtsp":
		c.rtspURL = allosResponse.Result.URL
		fmt.Printf("RTSP URL: %s\n", c.rtspURL)
	case "hls":
		c.hlsURL = allosResponse.Result.URL
		fmt.Printf("HLS URL: %s\n", c.hlsURL)
	default:
		return fmt.Errorf("unsupported stream type: %s", streamType)
	}

	return nil
}

func(c *TuyaClient) LoadHubConfig() (config *OpenIoTHubConfig, err error) {
	url := fmt.Sprintf("https://%s/v2.0/open-iot-hub/access/config", c.apiURL)

	request := &OpenIoTHubConfigRequest{
		UID:      c.uid,
		UniqueID: uuid.New().String(),
		LinkType: "mqtt",
		Topics:   "ipc",
	}

	body, err := c.Request("POST", url, request)
	if err != nil {
		return nil, fmt.Errorf("failed to get OpenIoTHub config: %w", err)
	}

	var openIoTHubConfigResponse OpenIoTHubConfigResponse
	err = json.Unmarshal(body, &openIoTHubConfigResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal OpenIoTHub config response: %w", err)
	}

	if !openIoTHubConfigResponse.Success {
		return nil, fmt.Errorf("failed to get OpenIoTHub config: %s", string(body))
	}
	
	return &openIoTHubConfigResponse.Result, nil
}

func(c *TuyaClient) calBusinessSign(ts int64) string {
	data := fmt.Sprintf("%s%s%s%d", c.clientID, c.accessToken, c.secret, ts)
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