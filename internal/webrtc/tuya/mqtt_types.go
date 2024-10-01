package tuya

type MqttFrameHeader struct {
	// mqtt消息类型，offer candidate answer disconnect
	Type string `json:"type"`

	// mqtt消息发送方
	From string `json:"from"`

	// mqtt消息接受方
	To string `json:"to"`

	// 如果发送方或接收方是设备，且是子设备，这里为子设备id
	SubDevID string `json:"sub_dev_id"`

	// mqtt消息所属的会话id
	SessionID string `json:"sessionid"`

	// mqtt消息相关的信令服务moto的id
	MotoID string `json:"moto_id"`

	// 事务id，MQTT控制信令透传时携带
	TransactionID string `json:"tid"`
}

// MqttFrame mqtt消息帧
type MqttFrame struct {
	Header  MqttFrameHeader `json:"header"`
	Message interface{}     `json:"msg"` // mqtt消息体，可为offer candidate answer disconnect，所以为interface{}
}

// MqttMessage mqtt消息（包含顶层协议头）
type MqttMessage struct {
	Protocol int       `json:"protocol"` // mqtt消息的协议号，webRTC属于实时流服务，为302
	Pv       string    `json:"pv"`       // 通讯协议版本号
	T        int64     `json:"t"`        // Unix时间戳，单位为second
	Data     MqttFrame `json:"data"`
}

type AnswerFrame struct {
	Mode string `json:"mode"`
	Sdp  string `json:"sdp"`
}

type CandidateFrame struct {
	Mode      string `json:"mode"`
	Candidate string `json:"candidate"`
}
