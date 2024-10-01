package tuya

import (
	"net/url"
)

type APPConfig struct {
	// Set by user
	OpenAPIURL string
	ClientID   string
	Secret     string
	UID        string
	DeviceID   string

	// Set by code
	MQTTUID      string
	AccessToken  string
	RefreshToken string
	ExpireTime   int64
}

var App = APPConfig{
	OpenAPIURL: "openapi.tuyaeu.com",
}

func LoadConfig(rawURL string, query url.Values) {
	App.OpenAPIURL = rawURL
	App.ClientID = query.Get("ClientID")
	App.Secret = query.Get("Secret")
	App.UID = query.Get("UID")
	App.DeviceID = query.Get("DeviceID")
}
