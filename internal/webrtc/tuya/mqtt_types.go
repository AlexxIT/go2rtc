package tuya

import "encoding/json"

type MqttFrameHeader struct {
	Type          string `json:"type"`
	From          string `json:"from"`
	To            string `json:"to"`
	SubDevID      string `json:"sub_dev_id"`
	SessionID     string `json:"sessionid"`
	MotoID        string `json:"moto_id"`
	TransactionID string `json:"tid"`
	Seq           int    `json:"seq"`
	Rtx           int    `json:"rtx"`
}

type MqttFrame struct {
	Header  MqttFrameHeader `json:"header"`
	Message json.RawMessage `json:"msg"`
}

type MqttMessage struct {
	Protocol int       `json:"protocol"`
	Pv       string    `json:"pv"`
	T        int64     `json:"t"`
	Data     MqttFrame `json:"data"`
}

type OfferFrame struct {
	Mode       string `json:"mode"`
	Sdp        string `json:"sdp"`
	StreamType uint32 `json:"stream_type"`
	Auth       string `json:"auth"`
}

type AnswerFrame struct {
	Mode string `json:"mode"`
	Sdp  string `json:"sdp"`
}

type CandidateFrame struct {
	Mode      string `json:"mode"`
	Candidate string `json:"candidate"`
}

type ResolutionFrame struct {
	Mode  string `json:"mode"`
	Value int    `json:"cmdValue"`
}

type ResolutionResultFrame struct {
	ResultCode int `json:"resCode"`
}
