package tuya

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type TuyaMqtt struct {
	client         mqtt.Client
	motoID         string
	auth           string
	iceServers     string
	publishTopic   string
	subscribeTopic string
	MQTTUID        string

	mqttReady core.Waiter

	handleAnswerFrame    func(answerFrame AnswerFrame)
	handleCandidateFrame func(candidateFrame CandidateFrame)
}

func (t *tuyaSession) StartMqtt() (err error) {
	t.mqtt.motoID, t.mqtt.auth, t.mqtt.iceServers, err = t.GetMotoIDAndAuth()
	if err != nil {
		log.Printf("GetMotoIDAndAuth fail: %s", err.Error())
		t.mqtt.mqttReady.Done(err)
		return
	}

	hubConfig, err := t.GetHubConfig()
	if err != nil {
		log.Printf("GetHubConfig fail: %s", err.Error())
		t.mqtt.mqttReady.Done(err)
		return
	}

	t.mqtt.publishTopic = hubConfig.SinkTopic.IPC
	t.mqtt.subscribeTopic = hubConfig.SourceSink.IPC

	t.mqtt.publishTopic = strings.Replace(t.mqtt.publishTopic, "moto_id", t.mqtt.motoID, 1)
	t.mqtt.publishTopic = strings.Replace(t.mqtt.publishTopic, "{device_id}", t.config.DeviceID, 1)

	log.Printf("publish topic: %s", t.mqtt.publishTopic)
	log.Printf("subscribe topic: %s", t.mqtt.subscribeTopic)

	parts := strings.Split(t.mqtt.subscribeTopic, "/")
	t.mqtt.MQTTUID = parts[3]

	opts := mqtt.NewClientOptions().AddBroker(hubConfig.Url).
		SetClientID(hubConfig.ClientID).
		SetUsername(hubConfig.Username).
		SetPassword(hubConfig.Password).
		SetOnConnectHandler((func(c mqtt.Client) {
			t.mqttOnConnect(c)
		})).
		SetConnectTimeout(10 * time.Second)

	t.mqtt.client = mqtt.NewClient(opts)
	if token := t.mqtt.client.Connect(); token.Wait() && token.Error() != nil {
		log.Printf("create mqtt client fail: %s", token.Error().Error())

		err = token.Error()
		t.mqtt.mqttReady.Done(err)
		return
	}

	return
}

func (t *tuyaSession) mqttOnConnect(client mqtt.Client) {
	options := client.OptionsReader()

	log.Printf("%s connect to mqtt success", options.ClientID())

	if token := client.Subscribe(t.mqtt.subscribeTopic, 1, func(c mqtt.Client, m mqtt.Message) {
		t.mqttConsume(m)
	}); token.Wait() && token.Error() != nil {
		log.Printf("subcribe fail: %s, topic: %s", token.Error().Error(), t.mqtt.subscribeTopic)
		t.mqtt.mqttReady.Done(token.Error())
		return
	}

	t.mqtt.mqttReady.Done(nil)

	log.Print("subscribe mqtt topic success")
}

func (t *tuyaSession) StopMqtt() {
	t.mqtt.client.Disconnect(1000)
}

func (t *tuyaSession) sendOffer(sessionID string, sdp string) {
	offerFrame := struct {
		Mode       string `json:"mode"`
		Sdp        string `json:"sdp"`
		StreamType uint32 `json:"stream_type"`
		Auth       string `json:"auth"`
	}{
		Mode:       "webrtc",
		Sdp:        sdp,
		StreamType: 1,
		Auth:       t.mqtt.auth,
	}

	offerMqtt := &MqttMessage{
		Protocol: 302,
		Pv:       "2.2",
		T:        time.Now().Unix(),
		Data: MqttFrame{
			Header: MqttFrameHeader{
				Type:      "offer",
				From:      t.mqtt.MQTTUID,
				To:        t.config.DeviceID,
				SubDevID:  "",
				SessionID: sessionID,
				MotoID:    t.mqtt.motoID,
			},
			Message: offerFrame,
		},
	}

	sendBytes, err := json.Marshal(offerMqtt)
	if err != nil {
		log.Printf("marshal offer mqtt to bytes fail: %s", err.Error())

		return
	}

	t.mqttPublish(sendBytes)
}

func (t *tuyaSession) sendCandidate(sessionID string, candidate string) {
	candidateFrame := struct {
		Mode      string `json:"mode"`
		Candidate string `json:"candidate"`
	}{
		Mode:      "webrtc",
		Candidate: candidate,
	}

	candidateMqtt := &MqttMessage{
		Protocol: 302,
		Pv:       "2.2",
		T:        time.Now().Unix(),
		Data: MqttFrame{
			Header: MqttFrameHeader{
				Type:      "candidate",
				From:      t.mqtt.MQTTUID,
				To:        t.config.DeviceID,
				SubDevID:  "",
				SessionID: sessionID,
				MotoID:    t.mqtt.motoID,
			},
			Message: candidateFrame,
		},
	}

	sendBytes, err := json.Marshal(candidateMqtt)
	if err != nil {
		log.Printf("marshal candidate mqtt to bytes fail: %s", err.Error())

		return
	}

	t.mqttPublish(sendBytes)
}

// 发布mqtt消息
func (t *tuyaSession) mqttPublish(payload []byte) {
	token := t.mqtt.client.Publish(t.mqtt.publishTopic, 1, false, payload)
	if token.Error() != nil {
		log.Printf("mqtt publish fail: %s, topic: %s", token.Error().Error(),
			t.mqtt.publishTopic)
	}
}

func (t *tuyaSession) mqttConsume(msg mqtt.Message) {
	tmp := struct {
		Protocol int    `json:"protocol"`
		Pv       string `json:"pv"`
		T        int64  `json:"t"`
		Data     struct {
			Header  MqttFrameHeader `json:"header"`
			Message json.RawMessage `json:"msg"`
		} `json:"data"`
	}{}

	if err := json.Unmarshal(msg.Payload(), &tmp); err != nil {
		log.Printf("unmarshal received mqtt fail: %s, payload: %s", err.Error(), string(msg.Payload()))

		return
	}

	rmqtt := &MqttMessage{
		Protocol: tmp.Protocol,
		Pv:       tmp.Pv,
		T:        tmp.T,
		Data: MqttFrame{
			Header:  tmp.Data.Header,
			Message: tmp.Data.Message,
		},
	}

	log.Printf("mqtt recv message, session: %s, type: %s, from: %s, to: %s",
		rmqtt.Data.Header.SessionID,
		rmqtt.Data.Header.Type,
		rmqtt.Data.Header.From,
		rmqtt.Data.Header.To)

	t.mqttDispatch(rmqtt)
}

// 分发从mqtt服务器接受到的消息
func (t *tuyaSession) mqttDispatch(msg *MqttMessage) {

	switch msg.Data.Header.Type {
	case "answer":
		t.mqttHandleAnswer(msg)
	case "candidate":
		t.mqttHandleCandidate(msg)
	}
}

func (t *tuyaSession) mqttHandleAnswer(msg *MqttMessage) {
	frame, ok := msg.Data.Message.(json.RawMessage)
	if !ok {
		log.Printf("convert interface{} to []byte fail, session: %s", msg.Data.Header.SessionID)

		return
	}

	answerFrame := AnswerFrame{}

	if err := json.Unmarshal(frame, &answerFrame); err != nil {
		log.Printf("unmarshal mqtt answer frame fail: %s, session: %s, frame: %s",
			err.Error(),
			msg.Data.Header.SessionID,
			string(msg.Data.Message.([]byte)))

		return
	}

	t.mqtt.handleAnswerFrame(answerFrame)
}

func (t *tuyaSession) mqttHandleCandidate(msg *MqttMessage) {
	frame, ok := msg.Data.Message.(json.RawMessage)
	if !ok {
		log.Printf("convert interface{} to []byte fail, session: %s", msg.Data.Header.SessionID)
		return
	}

	candidateFrame := CandidateFrame{}

	if err := json.Unmarshal(frame, &candidateFrame); err != nil {
		log.Printf("unmarshal mqtt candidate frame fail: %s, session: %s, frame: %s",
			err.Error(),
			msg.Data.Header.SessionID,
			string(msg.Data.Message.([]byte)))
		return
	}

	// candidate from device start with "a=", end with "\r\n", which are not needed by Pion webRTC
	candidateFrame.Candidate = strings.TrimPrefix(candidateFrame.Candidate, "a=")
	candidateFrame.Candidate = strings.TrimSuffix(candidateFrame.Candidate, "\r\n")

	t.mqtt.handleCandidateFrame(candidateFrame)
}
