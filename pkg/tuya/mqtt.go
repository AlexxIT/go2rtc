package tuya

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type TuyaMqttClient struct {
	client           mqtt.Client
	waiter           core.Waiter
	publishTopic     string
	subscribeTopic   string
	auth             string
	uid              string
	motoId           string
	deviceId         string
	sessionId        string
	closed           bool
	webrtcVersion    int
	handleAnswer     func(answer AnswerFrame)
	handleCandidate  func(candidate CandidateFrame)
	handleDisconnect func()
	handleError      func(err error)
}

type MqttFrameHeader struct {
	Type          string `json:"type"`
	From          string `json:"from"`
	To            string `json:"to"`
	SubDevID      string `json:"sub_dev_id"`
	SessionID     string `json:"sessionid"`
	MotoID        string `json:"moto_id"`
	TransactionID string `json:"tid"`
}

type MqttFrame struct {
	Header  MqttFrameHeader `json:"header"`
	Message json.RawMessage `json:"msg"`
}

type OfferFrame struct {
	Mode              string `json:"mode"`
	Sdp               string `json:"sdp"`
	StreamType        int    `json:"stream_type"`
	Auth              string `json:"auth"`
	DatachannelEnable bool   `json:"datachannel_enable"`
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
	Value int    `json:"cmdValue"` // 0: HD, 1: SD
}

type SpeakerFrame struct {
	Mode  string `json:"mode"`
	Value int    `json:"cmdValue"` // 0: off, 1: on
}

type DisconnectFrame struct {
	Mode string `json:"mode"`
}

type MqttMessage struct {
	Protocol int       `json:"protocol"`
	Pv       string    `json:"pv"`
	T        int64     `json:"t"`
	Data     MqttFrame `json:"data"`
}

func NewTuyaMqttClient(deviceId string) *TuyaMqttClient {
	return &TuyaMqttClient{
		deviceId:  deviceId,
		sessionId: core.RandString(6, 62),
		waiter:    core.Waiter{},
	}
}

func (c *TuyaMqttClient) Start(hubConfig *MQTTConfig, webrtcConfig *WebRTCConfig, webrtcVersion int) error {
	c.webrtcVersion = webrtcVersion
	c.motoId = webrtcConfig.MotoID
	c.auth = webrtcConfig.Auth

	c.publishTopic = hubConfig.PublishTopic
	c.subscribeTopic = hubConfig.SubscribeTopic

	c.publishTopic = strings.Replace(c.publishTopic, "moto_id", c.motoId, 1)
	c.publishTopic = strings.Replace(c.publishTopic, "{device_id}", c.deviceId, 1)

	parts := strings.Split(c.subscribeTopic, "/")
	c.uid = parts[3]

	opts := mqtt.NewClientOptions().AddBroker(hubConfig.Url).
		SetClientID(hubConfig.ClientID).
		SetUsername(hubConfig.Username).
		SetPassword(hubConfig.Password).
		SetOnConnectHandler(c.onConnect).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(30 * time.Second).
		SetConnectTimeout(15 * time.Second).
		SetKeepAlive(30 * time.Second).
		SetPingTimeout(15 * time.Second)

	c.client = mqtt.NewClient(opts)

	if token := c.client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	if err := c.waiter.Wait(); err != nil {
		return err
	}

	return nil
}

func (c *TuyaMqttClient) Stop() {
	if c.client != nil {
		_ = c.SendDisconnect()
		c.client.Disconnect(1000)
	}
}

func (c *TuyaMqttClient) SendOffer(sdp string, streamResolution string, streamType int, isHEVC bool) error {
	if isHEVC {
		// On HEVC we use streamType 0 for main stream (hd) and 1 for sub stream (sd)
		if streamResolution == "hd" {
			streamType = 0
		} else {
			streamType = 1
		}
	}

	return c.sendMqttMessage("offer", 302, "", OfferFrame{
		Mode:              "webrtc",
		Sdp:               sdp,
		StreamType:        streamType,
		Auth:              c.auth,
		DatachannelEnable: isHEVC,
	})
}

func (c *TuyaMqttClient) SendCandidate(candidate string) error {
	return c.sendMqttMessage("candidate", 302, "", CandidateFrame{
		Mode:      "webrtc",
		Candidate: candidate,
	})
}

func (c *TuyaMqttClient) SendResolution(resolution int) error {
	// isClaritySupperted := (c.webrtcVersion & (1 << 5)) != 0
	// if !isClaritySupperted {
	// 	return nil
	// }

	// Protocol 312 is used for clarity
	return c.sendMqttMessage("resolution", 312, "", ResolutionFrame{
		Mode:  "webrtc",
		Value: resolution,
	})
}

func (c *TuyaMqttClient) SendSpeaker(speaker int) error {
	// Protocol 312 is used for speaker
	return c.sendMqttMessage("speaker", 312, "", SpeakerFrame{
		Mode:  "webrtc",
		Value: speaker,
	})
}

func (c *TuyaMqttClient) SendDisconnect() error {
	return c.sendMqttMessage("disconnect", 302, "", DisconnectFrame{
		Mode: "webrtc",
	})
}

func (c *TuyaMqttClient) onConnect(client mqtt.Client) {
	if token := client.Subscribe(c.subscribeTopic, 1, c.consume); token.Wait() && token.Error() != nil {
		c.waiter.Done(token.Error())
		return
	}

	c.waiter.Done(nil)
}

func (c *TuyaMqttClient) consume(client mqtt.Client, msg mqtt.Message) {
	var rmqtt MqttMessage
	if err := json.Unmarshal(msg.Payload(), &rmqtt); err != nil {
		c.onError(err)
		return
	}

	if rmqtt.Data.Header.SessionID != c.sessionId {
		return
	}

	switch rmqtt.Data.Header.Type {
	case "answer":
		c.onMqttAnswer(&rmqtt)
	case "candidate":
		c.onMqttCandidate(&rmqtt)
	case "disconnect":
		c.onMqttDisconnect()
	}
}

func (c *TuyaMqttClient) onMqttAnswer(msg *MqttMessage) {
	var answerFrame AnswerFrame
	if err := json.Unmarshal(msg.Data.Message, &answerFrame); err != nil {
		c.onError(err)
		return
	}

	c.onAnswer(answerFrame)
}

func (c *TuyaMqttClient) onMqttCandidate(msg *MqttMessage) {
	var candidateFrame CandidateFrame
	if err := json.Unmarshal(msg.Data.Message, &candidateFrame); err != nil {
		c.onError(err)
		return
	}

	// fix candidates
	candidateFrame.Candidate = strings.TrimPrefix(candidateFrame.Candidate, "a=")
	candidateFrame.Candidate = strings.TrimSuffix(candidateFrame.Candidate, "\r\n")

	c.onCandidate(candidateFrame)
}

func (c *TuyaMqttClient) onMqttDisconnect() {
	c.closed = true
	c.onDisconnect()
}

func (c *TuyaMqttClient) onAnswer(answer AnswerFrame) {
	if c.handleAnswer != nil {
		c.handleAnswer(answer)
	}
}

func (c *TuyaMqttClient) onCandidate(candidate CandidateFrame) {
	if c.handleCandidate != nil {
		c.handleCandidate(candidate)
	}
}

func (c *TuyaMqttClient) onDisconnect() {
	if c.handleDisconnect != nil {
		c.handleDisconnect()
	}
}

func (c *TuyaMqttClient) onError(err error) {
	if c.handleError != nil {
		c.handleError(err)
	}
}

func (c *TuyaMqttClient) sendMqttMessage(messageType string, protocol int, transactionID string, data interface{}) error {
	if c.closed {
		return fmt.Errorf("mqtt client is closed, send mqtt message fail")
	}

	jsonMessage, err := json.Marshal(data)
	if err != nil {
		return err
	}

	msg := &MqttMessage{
		Protocol: protocol,
		Pv:       "2.2",
		T:        time.Now().Unix(),
		Data: MqttFrame{
			Header: MqttFrameHeader{
				Type:          messageType,
				From:          c.uid,
				To:            c.deviceId,
				SessionID:     c.sessionId,
				MotoID:        c.motoId,
				TransactionID: transactionID,
			},
			Message: jsonMessage,
		},
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	token := c.client.Publish(c.publishTopic, 1, false, payload)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}

	return nil
}
