package tuya

type OpenIoTHubConfig struct {
	Url      string `json:"url"`       // MQTT connection address
	ClientID string `json:"client_id"` // MQTT connection client_id
	Username string `json:"username"`  // MQTT connection username
	Password string `json:"password"`  // MQTT connection password

	// Publishing topic, used to control the device via this topic
	SinkTopic struct {
		IPC string `json:"ipc"`
	} `json:"sink_topic"`

	// Subscription topic, used for device events and device status synchronization
	SourceSink struct {
		IPC string `json:"ipc"`
	} `json:"source_topic"`

	ExpireTime int `json:"expire_time"` // Validity period of the current configuration. Once expired, all connections will be disconnected.
}

// OpenIoTHubConfigRequest HTTP request body to get for MQTT connection config
type OpenIoTHubConfigRequest struct {
	UID      string `json:"uid"`       // Tuya user ID
	UniqueID string `json:"unique_id"` // Connection identifier, used to separate different connections, generated on client.
	LinkType string `json:"link_type"` // Connection type, currently only supports MQTT
	Topics   string `json:"topics"`    // Subscribed MQTT topic, this sample only focuses on the IPC topic
}

// Token ICE Token from OpenAPI
type Token struct {
	Urls       string `json:"urls"`
	Username   string `json:"username"`
	Credential string `json:"credential"`
	TTL        int    `json:"ttl"`
}

// WebToken ICE Token to Chrome
type WebToken struct {
	Urls       string `json:"urls,omitempty"`
	Username   string `json:"username,omitempty"`
	Credential string `json:"credential,omitempty"`
}
