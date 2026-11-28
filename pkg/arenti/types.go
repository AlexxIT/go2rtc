package arenti

import "encoding/json"

// LoginResponse represents the response from /ipc_web/user/login
type LoginResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		UserID      int64  `json:"userID"`
		UserAccount string `json:"userAccount"`
		UserToken   string `json:"userToken"`
		WssDomain   string `json:"wssDomain"`
		CountryCode string `json:"countryCode"`
		PhoneCode   string `json:"phoneCode"`
	} `json:"data"`
}

// DeviceListResponse represents the response from /ipc_web/device/list
type DeviceListResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Ipc  []Device `json:"ipc"`
		Nvr  []Device `json:"nvr"`
		Snap []Device `json:"snap"`
	} `json:"data"`
}

// Device represents an Arenti/Meari camera device
type Device struct {
	DeviceID       int64  `json:"deviceID"`
	DeviceName     string `json:"deviceName"`
	SnNum          string `json:"snNum"`
	HostKey        string `json:"hostKey"`
	P2PID          string `json:"p2pID"`
	Model          string `json:"model"`
	Category       string `json:"category"`
	Capability     string `json:"capability"`
	IsOwner        int    `json:"isOwner"`
	IconURL        string `json:"iconUrl"`
	DeviceTypeName string `json:"deviceTypeName"`
	Version        string `json:"version"`
	UpgradeVer     string `json:"upgradeVer"`
	WifiStrength   int    `json:"wifiStrength"`
	Battery        int    `json:"battery"`
	IP             string `json:"ip"`
}

// DeviceStatusResponse represents the response from /ipc_web/iot/query_device_status
type DeviceStatusResponse struct {
	Code string         `json:"code"`
	Msg  string         `json:"msg"`
	Data []DeviceStatus `json:"data"`
}

// DeviceStatus represents runtime device connectivity state
type DeviceStatus struct {
	DeviceID string `json:"deviceid"`
	Status   string `json:"status"` // "dormancy" | "online" | "offline"
	Runtime  int    `json:"runtime"`
}

// SignWssResponse represents the response from /ipc_web/iot_sign/wss
type SignWssResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		AccessID  string `json:"accessid"`
		Signature string `json:"signature"`
		Token     string `json:"token"`
	} `json:"data"`
}

// MtsMessage represents an MTS envelope on WebSocket
type MtsMessage struct {
	Action string          `json:"action"` // "req" | "rsp"
	Cmd    string          `json:"cmd"`    // "mts"
	Method string          `json:"method"` // "hello" | "option" | "offer" | "answer" | "close" | "candidate"
	Sid    string          `json:"sid"`
	Auth   *MtsAuth        `json:"auth,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

// MtsAuth carries the signaling ticket
type MtsAuth struct {
	AccessID  string `json:"accessid"`
	Signature string `json:"signature"`
	Token     string `json:"token"`
}

// MtsOptionParams are sent in the "option" request
type MtsOptionParams struct {
	Caller     string `json:"caller"`
	Callee     string `json:"callee"`
	DeviceCode string `json:"devicecode"`
	Expires    string `json:"expires"`
}

// TurnCredentials returned in "option" response
type TurnCredentials struct {
	CoturnHost string `json:"coturn_host"`
	CoturnIP   string `json:"coturn_ip"`
	CoturnPort int    `json:"coturn_port"`
	Username   string `json:"username"`
	Password   string `json:"pwd"`
}

// MtsStream defines requested stream channel
type MtsStream struct {
	Channel int `json:"channel"`
	Stream  int `json:"stream"`
}

// MtsSettings contains stream configuration in offer
type MtsSettings struct {
	Streams []MtsStream `json:"streams"`
}

// MtsOfferParams sent in "offer" request
type MtsOfferParams struct {
	Caller     string      `json:"caller"`
	Callee     string      `json:"callee"`
	DeviceCode string      `json:"devicecode"`
	SDP        string      `json:"sdp"`
	Settings   MtsSettings `json:"settings"`
}

// MtsAnswerParams received in "answer" response
type MtsAnswerParams struct {
	Caller     string `json:"caller,omitempty"`
	Callee     string `json:"callee,omitempty"`
	DeviceCode string `json:"devicecode,omitempty"`
	SDP        string `json:"sdp"`
}

// MtsCloseParams sent in "close" request
type MtsCloseParams struct {
	Caller     string `json:"caller"`
	Callee     string `json:"callee"`
	DeviceCode string `json:"devicecode"`
}
