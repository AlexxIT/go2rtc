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

type ResolutionFrame struct {
	Mode  string `json:"mode"`
	Value int    `json:"value"` // 0: HD, 1: SD
}

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
		c.sendDisconnect()
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
		c.mqtt.onError(fmt.Errorf("unmarshal mqtt message fail: %s, payload: %s", err.Error(), string(msg.Payload())))
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
		c.onError(fmt.Errorf("unmarshal mqtt answer frame fail: %s, session: %s, frame: %s",
			err.Error(),
			msg.Data.Header.SessionID,
			string(msg.Data.Message)))
		return
	}

	c.onAnswer(answerFrame)
}

func (c *TuyaMQTT) onMqttCandidate(msg *MqttMessage) {
	var candidateFrame CandidateFrame
	if err := json.Unmarshal(msg.Data.Message, &candidateFrame); err != nil {
		c.onError(fmt.Errorf("unmarshal mqtt candidate frame fail: %s, session: %s, frame: %s",
			err.Error(),
			msg.Data.Header.SessionID,
			string(msg.Data.Message)))
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

func (c *TuyaClient) sendOffer(sdp string, streamType int) {
	// H265 is currently not supported because Tuya does not send H265 data, and therefore also no audio over the normal WebRTC connection.
	// The WebRTC connection is used only for sending audio back to the device (backchannel).
	// Tuya expects a separate WebRTC DataChannel for H265 data and sends the H265 video and audio data packaged as fMP4 data back.
	// These must then be processed separately (WIP - Work In Progress)

	// Example Answer (H265/PCMU with backchannel):

	/*
	   v=0
	   o=- 1747174385 1 IN IP4 127.0.0.1
	   s=-
	   t=0 0
	   a=group:BUNDLE 0 1
	   a=msid-semantic: WMS UMSklk
	   m=audio 9 UDP/TLS/RTP/SAVPF 0
	   c=IN IP4 0.0.0.0
	   a=rtcp:9 IN IP4 0.0.0.0
	   a=ice-ufrag:zuRr
	   a=ice-pwd:EDeWXz847P810fyDyKxbmTdX
	   a=ice-options:trickle
	   a=fingerprint:sha-256 02:f5:44:8e:c6:5d:5c:59:49:50:a3:84:d5:e5:b9:35:bb:51:5a:0c:4d:a5:60:89:0f:e6:cb:0e:57:21:a0:14
	   a=setup:active
	   a=mid:0
	   a=sendrecv
	   a=msid:UMSklk NiNNboEn1rJWoQYtpguoKr1GBwpvPST
	   a=rtcp-mux
	   a=rtpmap:0 PCMU/8000
	   a=ssrc:832759612 cname:bfa87264438073154dhdek
	   m=video 9 UDP/TLS/RTP/SAVPF 0
	   c=IN IP4 0.0.0.0
	   a=rtcp:9 IN IP4 0.0.0.0
	   a=ice-ufrag:zuRr
	   a=ice-pwd:EDeWXz847P810fyDyKxbmTdX
	   a=ice-options:trickle
	   a=fingerprint:sha-256 02:f5:44:8e:c6:5d:5c:59:49:50:a3:84:d5:e5:b9:35:bb:51:5a:0c:4d:a5:60:89:0f:e6:cb:0e:57:21:a0:14
	   a=setup:active
	   a=mid:1
	   a=sendonly
	   a=msid:UMSklk l9o6icIVb7n7vDdp0KhocYnsijhd774
	   a=rtcp-mux
	   a=rtpmap:0 /0
	   a=rtcp-fb:0 ccm fir
	   a=rtcp-fb:0 nack
	   a=rtcp-fb:0 nack pli
	   a=fmtp:0 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=
	   a=ssrc:0 cname:bfa87264438073154dhdek
	*/

	c.sendMqttMessage("offer", 302, "", OfferFrame{
		Mode:              "webrtc",
		Sdp:               sdp,
		StreamType:        streamType,
		Auth:              c.auth,
		DatachannelEnable: c.isHEVC(streamType),
	})
}

func (c *TuyaClient) sendCandidate(candidate string) {
	c.sendMqttMessage("candidate", 302, "", CandidateFrame{
		Mode:      "webrtc",
		Candidate: candidate,
	})
}

func (c *TuyaClient) sendResolution(resolution int) {
	if !c.isClaritySupported(resolution) {
		return
	}

	c.sendMqttMessage("resolution", 302, "", ResolutionFrame{
		Mode:  "webrtc",
		Value: resolution,
	})
}

func (c *TuyaClient) sendSpeaker(speaker int) {
	c.sendMqttMessage("speaker", 302, "", SpeakerFrame{
		Mode:  "webrtc",
		Value: speaker,
	})
}

func (c *TuyaClient) sendDisconnect() {
	c.sendMqttMessage("disconnect", 302, "", DisconnectFrame{
		Mode: "webrtc",
	})
}

func (c *TuyaClient) sendMqttMessage(messageType string, protocol int, transactionID string, data interface{}) {
	if c.mqtt.closed {
		c.mqtt.onError(fmt.Errorf("mqtt client is closed, send mqtt message fail"))
		return
	}

	jsonMessage, err := json.Marshal(data)
	if err != nil {
		c.mqtt.onError(fmt.Errorf("marshal mqtt message fail: %s", err.Error()))
		return
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
		c.mqtt.onError(fmt.Errorf("marshal mqtt message fail: %s", err.Error()))
		return
	}

	token := c.mqtt.client.Publish(c.mqtt.publishTopic, 1, false, payload)
	if token.Wait() && token.Error() != nil {
		c.mqtt.onError(fmt.Errorf("mqtt publish fail: %s, topic: %s", token.Error().Error(), c.mqtt.publishTopic))
	}
}

func (c *TuyaClient) isHEVC(streamType int) bool {
	for _, video := range c.skill.Videos {
		if video.StreamType == streamType {
			return video.CodecType == 4
		}
	}

	return false
}

func (c *TuyaClient) isClaritySupported(webrtcValue int) bool {
	return (webrtcValue & (1 << 5)) != 0
}
