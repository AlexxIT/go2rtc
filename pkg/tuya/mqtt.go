package tuya

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type TuyaMQTT struct {
	client           mqtt.Client
	waiter           core.Waiter
	publishTopic     string
	subscribeTopic   string
	uid              string
	closed           bool
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

// type ResolutionFrame struct {
// 	Mode  string `json:"mode"`
// 	Value int    `json:"value"` // 0: HD, 1: SD
// }

type SpeakerFrame struct {
	Mode  string `json:"mode"`
	Value int    `json:"value"` // 0: off, 1: on
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

func (c *TuyaClient) StartMQTT() error {
	hubConfig, err := c.LoadHubConfig()
	if err != nil {
		return err
	}

	c.mqtt.publishTopic = hubConfig.SinkTopic.IPC
	c.mqtt.subscribeTopic = hubConfig.SourceSink.IPC

	c.mqtt.publishTopic = strings.Replace(c.mqtt.publishTopic, "moto_id", c.motoId, 1)
	c.mqtt.publishTopic = strings.Replace(c.mqtt.publishTopic, "{device_id}", c.deviceId, 1)

	parts := strings.Split(c.mqtt.subscribeTopic, "/")
	c.mqtt.uid = parts[3]

	opts := mqtt.NewClientOptions().AddBroker(hubConfig.Url).
		SetClientID(hubConfig.ClientID).
		SetUsername(hubConfig.Username).
		SetPassword(hubConfig.Password).
		SetOnConnectHandler(c.onConnect).
		SetConnectTimeout(10 * time.Second)

	c.mqtt.client = mqtt.NewClient(opts)

	if token := c.mqtt.client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	if err := c.mqtt.waiter.Wait(); err != nil {
		return err
	}

	return nil
}

func (c *TuyaClient) StopMQTT() {
	if c.mqtt.client != nil {
		_ = c.sendDisconnect()
		c.mqtt.client.Disconnect(1000)
	}
}

func (c *TuyaClient) onConnect(client mqtt.Client) {
	if token := client.Subscribe(c.mqtt.subscribeTopic, 1, c.consume); token.Wait() && token.Error() != nil {
		c.mqtt.waiter.Done(token.Error())
		return
	}

	c.mqtt.waiter.Done(nil)
}

func (c *TuyaClient) consume(client mqtt.Client, msg mqtt.Message) {
	var rmqtt MqttMessage
	if err := json.Unmarshal(msg.Payload(), &rmqtt); err != nil {
		c.mqtt.onError(err)
		return
	}

	if rmqtt.Data.Header.SessionID != c.sessionId {
		return
	}

	switch rmqtt.Data.Header.Type {
	case "answer":
		c.mqtt.onMqttAnswer(&rmqtt)
	case "candidate":
		c.mqtt.onMqttCandidate(&rmqtt)
	case "disconnect":
		c.mqtt.onMqttDisconnect()
	}
}

func (c *TuyaMQTT) onMqttAnswer(msg *MqttMessage) {
	var answerFrame AnswerFrame
	if err := json.Unmarshal(msg.Data.Message, &answerFrame); err != nil {
		c.onError(err)
		return
	}

	c.onAnswer(answerFrame)
}

func (c *TuyaMQTT) onMqttCandidate(msg *MqttMessage) {
	var candidateFrame CandidateFrame
	if err := json.Unmarshal(msg.Data.Message, &candidateFrame); err != nil {
		c.onError(err)
		return
	}

	// candidate from device start with "a=", end with "\r\n", which are not needed by Chrome webRTC
	candidateFrame.Candidate = strings.TrimPrefix(candidateFrame.Candidate, "a=")
	candidateFrame.Candidate = strings.TrimSuffix(candidateFrame.Candidate, "\r\n")

	c.onCandidate(candidateFrame)
}

func (c *TuyaMQTT) onMqttDisconnect() {
	c.closed = true
	c.onDisconnect()
}

func (c *TuyaMQTT) onAnswer(answer AnswerFrame) {
	if c.handleAnswer != nil {
		c.handleAnswer(answer)
	}
}

func (c *TuyaMQTT) onCandidate(candidate CandidateFrame) {
	if c.handleCandidate != nil {
		c.handleCandidate(candidate)
	}
}

func (c *TuyaMQTT) onDisconnect() {
	if c.handleDisconnect != nil {
		c.handleDisconnect()
	}
}

func (c *TuyaMQTT) onError(err error) {
	if c.handleError != nil {
		c.handleError(err)
	}
}

func (c *TuyaClient) sendOffer(sdp string, streamRole string) error {
	streamType := c.getStreamType(streamRole)
	isHEVC := c.isHEVC(streamType)

	if isHEVC {
		// On HEVC we use streamType 0 for main stream and 1 for sub stream
		if streamRole == "main" {
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

func (c *TuyaClient) sendCandidate(candidate string) error {
	return c.sendMqttMessage("candidate", 302, "", CandidateFrame{
		Mode:      "webrtc",
		Candidate: candidate,
	})
}

// func (c *TuyaClient) sendResolution(resolution int) error {
// 	isClaritySupperted := (c.skill.WebRTC & (1 << 5)) != 0
// 	if !isClaritySupperted {
// 		return nil
// 	}

// 	return c.sendMqttMessage("resolution", 302, "", ResolutionFrame{
// 		Mode:  "webrtc",
// 		Value: resolution,
// 	})
// }

func (c *TuyaClient) sendSpeaker(speaker int) error {
	return c.sendMqttMessage("speaker", 302, "", SpeakerFrame{
		Mode:  "webrtc",
		Value: speaker,
	})
}

func (c *TuyaClient) sendDisconnect() error {
	return c.sendMqttMessage("disconnect", 302, "", DisconnectFrame{
		Mode: "webrtc",
	})
}

func (c *TuyaClient) sendMqttMessage(messageType string, protocol int, transactionID string, data interface{}) error {
	if c.mqtt.closed {
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
				From:          c.mqtt.uid,
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

	token := c.mqtt.client.Publish(c.mqtt.publishTopic, 1, false, payload)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}

	return nil
}
