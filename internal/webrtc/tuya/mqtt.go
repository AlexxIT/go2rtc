package tuya

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
)

type TuyaMqtt struct {
	client           mqtt.Client
	motoID           string
	auth             string
	iceServers       string
	publishTopic     string
	subscribeTopic   string
	MQTTUID          string
	resolutionSet    bool
	resolutionResult core.Waiter

	mqttReady core.Waiter

	handleAnswerFrame    func(answerFrame AnswerFrame)
	handleCandidateFrame func(candidateFrame CandidateFrame)
}

func (t *tuyaSession) StartMqtt() (err error) {
	t.mqtt.motoID, t.mqtt.auth, t.mqtt.iceServers, err = t.GetMotoIDAndAuth()
	if err != nil {
		log.Error().Msgf("GetMotoIDAndAuth fail: %s", err.Error())
		t.mqtt.mqttReady.Done(err)
		return
	}

	hubConfig, err := t.GetHubConfig()
	if err != nil {
		log.Error().Msgf("GetHubConfig fail: %s", err.Error())
		t.mqtt.mqttReady.Done(err)
		return
	}

	t.mqtt.publishTopic = hubConfig.SinkTopic.IPC
	t.mqtt.subscribeTopic = hubConfig.SourceSink.IPC

	t.mqtt.publishTopic = strings.Replace(t.mqtt.publishTopic, "moto_id", t.mqtt.motoID, 1)
	t.mqtt.publishTopic = strings.Replace(t.mqtt.publishTopic, "{device_id}", t.config.DeviceID, 1)

	log.Debug().Msgf("publish topic: %s", t.mqtt.publishTopic)
	log.Debug().Msgf("subscribe topic: %s", t.mqtt.subscribeTopic)

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
		log.Error().Msgf("create mqtt client fail: %s", token.Error().Error())

		err = token.Error()
		t.mqtt.mqttReady.Done(err)
		return
	}

	return
}

func (t *tuyaSession) mqttOnConnect(client mqtt.Client) {
	options := client.OptionsReader()

	log.Debug().Msgf("%s connect to mqtt success", options.ClientID())

	if token := client.Subscribe(t.mqtt.subscribeTopic, 1, func(c mqtt.Client, m mqtt.Message) {
		t.mqttConsume(m)
	}); token.Wait() && token.Error() != nil {
		log.Error().Msgf("subcribe fail: %s, topic: %s", token.Error().Error(), t.mqtt.subscribeTopic)
		t.mqtt.mqttReady.Done(token.Error())
		return
	}

	t.mqtt.mqttReady.Done(nil)

	log.Debug().Msgf("subscribe mqtt topic success")
}

func (t *tuyaSession) StopMqtt() {
	t.mqtt.client.Disconnect(1000)
}

func (t *tuyaSession) sendMqttMessage(messageType string, protocol int, transactionID string, data interface{}) {

	jsonMessage, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Failed to marshal message:", err)
		return
	}

	msg := &MqttMessage{
		Protocol: protocol,
		Pv:       "2.2",
		T:        time.Now().Unix(),
		Data: MqttFrame{
			Header: MqttFrameHeader{
				Type:          messageType,
				From:          t.mqtt.MQTTUID,
				To:            t.config.DeviceID,
				SessionID:     t.sessionId,
				MotoID:        t.mqtt.motoID,
				TransactionID: transactionID,
			},
			Message: jsonMessage,
		},
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		log.Error().Msgf("Failed to marshal %s message: %v", messageType, err)
		return
	}

	log.Debug().Msgf("mqtt send message, session: %s, type: %s, from: %s, to: %s, message %s",
		msg.Data.Header.SessionID,
		msg.Data.Header.Type,
		msg.Data.Header.From,
		msg.Data.Header.To,
		string(msg.Data.Message),
	)

	t.mqttPublish(payload)
}

func (t *tuyaSession) sendOffer(sdp string) {
	t.sendMqttMessage("offer", 302, "", OfferFrame{
		Mode:       "webrtc",
		Sdp:        sdp,
		StreamType: 1,
		Auth:       t.mqtt.auth,
	})
}

func (t *tuyaSession) sendCandidate(candidate string) {
	t.sendMqttMessage("candidate", 302, "", CandidateFrame{
		Mode:      "webrtc",
		Candidate: candidate,
	})
}

func (t *tuyaSession) sendResolution(resolution int) {
	t.mqtt.resolutionResult = core.Waiter{}
	t.sendMqttMessage("resolution", 312, uuid.New().String(), ResolutionFrame{
		Mode:  "webrtc",
		Value: resolution,
	})
}

func (t *tuyaSession) mqttPublish(payload []byte) {
	token := t.mqtt.client.Publish(t.mqtt.publishTopic, 1, false, payload)
	if token.Wait() && token.Error() != nil {
		log.Error().Msgf("mqtt publish fail: %s, topic: %s", token.Error().Error(),
			t.mqtt.publishTopic)
	}
}

func (t *tuyaSession) mqttConsume(msg mqtt.Message) {
	rmqtt := MqttMessage{}

	if err := json.Unmarshal(msg.Payload(), &rmqtt); err != nil {
		log.Error().Msgf("unmarshal received mqtt fail: %s, payload: %s", err.Error(), string(msg.Payload()))

		return
	}

	log.Debug().Msgf("mqtt recv message, session: %s, type: %s, from: %s, to: %s, message %s",
		rmqtt.Data.Header.SessionID,
		rmqtt.Data.Header.Type,
		rmqtt.Data.Header.From,
		rmqtt.Data.Header.To,
		string(rmqtt.Data.Message),
	)

	t.mqttDispatch(&rmqtt)
}

func (t *tuyaSession) mqttDispatch(msg *MqttMessage) {

	switch msg.Data.Header.Type {
	case "answer":
		t.mqttHandleAnswer(msg)
	case "candidate":
		t.mqttHandleCandidate(msg)
	case "resolution":
		t.mqttHandleResolution(msg)
	}
}

func (t *tuyaSession) mqttHandleAnswer(msg *MqttMessage) {

	answerFrame := AnswerFrame{}

	if err := json.Unmarshal(msg.Data.Message, &answerFrame); err != nil {
		log.Error().Msgf("unmarshal mqtt answer frame fail: %s, session: %s, frame: %s",
			err.Error(),
			msg.Data.Header.SessionID,
			string(msg.Data.Message))
		return
	}

	t.mqtt.handleAnswerFrame(answerFrame)
}

func (t *tuyaSession) mqttHandleCandidate(msg *MqttMessage) {

	candidateFrame := CandidateFrame{}

	if err := json.Unmarshal(msg.Data.Message, &candidateFrame); err != nil {
		log.Error().Msgf("unmarshal mqtt candidate frame fail: %s, session: %s, frame: %s",
			err.Error(),
			msg.Data.Header.SessionID,
			string(msg.Data.Message))
		return
	}

	// candidate from device start with "a=", end with "\r\n", which are not needed by Pion webRTC
	candidateFrame.Candidate = strings.TrimPrefix(candidateFrame.Candidate, "a=")
	candidateFrame.Candidate = strings.TrimSuffix(candidateFrame.Candidate, "\r\n")

	t.mqtt.handleCandidateFrame(candidateFrame)
}

func (t *tuyaSession) mqttHandleResolution(msg *MqttMessage) {

	resultFrame := ResolutionResultFrame{}

	if err := json.Unmarshal(msg.Data.Message, &resultFrame); err != nil {
		log.Error().Msgf("unmarshal mqtt resolution result frame fail: %s, session: %s, frame: %s",
			err.Error(),
			msg.Data.Header.SessionID,
			string(msg.Data.Message))
		t.mqtt.resolutionResult.Done(err)
		return
	}

	t.mqtt.resolutionSet = resultFrame.ResultCode == 0
	t.mqtt.resolutionResult.Done(nil)
}
