package tuya

type OpenIoTHubConfig struct {
	Url      string `json:"url"`       // mqtt连接地址（包括protocol、ip、port）
	ClientID string `json:"client_id"` // mqtt连接client_id（用户账号及unique_id 生成的一个唯一不变的映射）一个clientId 即可以用于发布也可以订阅
	Username string `json:"username"`  // mqtt连接用户名（用户账号生成的一个唯一不变的映射）
	Password string `json:"password"`  // mqtt连接密码 ，失效期内该字段不变

	// 发布topic，控制设备可通过该topic完成
	SinkTopic struct {
		IPC string `json:"ipc"`
	} `json:"sink_topic"`

	// 订阅topic，设备事件、设备状态同步，可以订阅该topic
	SourceSink struct {
		IPC string `json:"ipc"`
	} `json:"source_topic"`

	ExpireTime int `json:"expire_time"` // 当前配置有效时长，当前配置失效后所有的连接都将断开
}

// OpenIoTHubConfigRequest 向开放平台申请mqtt连接的http请求体
type OpenIoTHubConfigRequest struct {
	UID      string `json:"uid"`       // 涂鸦用户id
	UniqueID string `json:"unique_id"` // 连接端按unique_id隔离，当同一用户需要在多端登录时，调用方需要保证unique_id不同
	LinkType string `json:"link_type"` // 连接类型，暂只支持mqtt
	Topics   string `json:"topics"`    // 关注的mqtt topic，本Sample只关注ipc topic
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
