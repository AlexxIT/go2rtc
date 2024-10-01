package tuya

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var (
	client mqtt.Client

	motoID string
	auth   string

	iceServers string

	publishTopic   string
	subscribeTopic string
)

func StartMqtt(
	handleAnswerFrame func(answerFrame AnswerFrame),
	handleCandidateFrame func(candidateFrame CandidateFrame),
) (err error) {
	motoID, auth, iceServers, err = GetMotoIDAndAuth()
	if err != nil {
		log.Printf("allocate motoID fail: %s", err.Error())

		return
	}

	log.Printf("motoID: %s", motoID)
	log.Printf("auth: %s", auth)
	log.Printf("iceServers: %s", iceServers)

	hubConfig, err := LoadHubConfig()
	if err != nil {
		log.Printf("loadConfig fail: %s", err.Error())

		return
	}

	log.Printf("hubConfig: %+v", *hubConfig)

	publishTopic = hubConfig.SinkTopic.IPC
	subscribeTopic = hubConfig.SourceSink.IPC

	publishTopic = strings.Replace(publishTopic, "moto_id", motoID, 1)
	publishTopic = strings.Replace(publishTopic, "{device_id}", App.DeviceID, 1)

	log.Printf("publish topic: %s", publishTopic)
	log.Printf("subscribe topic: %s", subscribeTopic)

	parts := strings.Split(subscribeTopic, "/")
	App.MQTTUID = parts[3]

	opts := mqtt.NewClientOptions().AddBroker(hubConfig.Url).
		SetClientID(hubConfig.ClientID).
		SetUsername(hubConfig.Username).
		SetPassword(hubConfig.Password).
		SetOnConnectHandler((func(c mqtt.Client) {
			onConnect(c, handleAnswerFrame, handleCandidateFrame)
		})).
		SetConnectTimeout(10 * time.Second)

	client = mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Printf("create mqtt client fail: %s", token.Error().Error())

		err = token.Error()
		return
	}

	return
}

func FetchWebRTCConfigs() (err error) {
	_, _, iceServers, err = GetMotoIDAndAuth()
	if err != nil {
		log.Printf("get webrtc configs fail: %s", err.Error())

		return err
	}

	log.Printf("iceServers: %s", iceServers)

	return nil
}

func IceServers() string {
	return iceServers
}

func onConnect(client mqtt.Client,
	handleAnswerFrame func(answerFrame AnswerFrame),
	handleCandidateFrame func(candidateFrame CandidateFrame),
) {
	options := client.OptionsReader()

	log.Printf("%s connect to mqtt success", options.ClientID())

	if token := client.Subscribe(subscribeTopic, 1, func(c mqtt.Client, m mqtt.Message) {
		consume(c, m, handleAnswerFrame, handleCandidateFrame)
	}); token.Wait() && token.Error() != nil {
		log.Printf("subcribe fail: %s, topic: %s", token.Error().Error(), subscribeTopic)

		return
	}

	log.Print("subscribe mqtt topic success")
}

func Unsubscribe() {
	if token := client.Unsubscribe(subscribeTopic); token.Wait() && token.Error() != nil {
		log.Printf("unsubscribe fail: %s, topic: %s", token.Error().Error(), subscribeTopic)
	}
}

func Disconnect() {
	client.Disconnect(1000)
}

func sendOffer(sessionID string, sdp string) {
	offerFrame := struct {
		Mode       string `json:"mode"`
		Sdp        string `json:"sdp"`
		StreamType uint32 `json:"stream_type"`
		Auth       string `json:"auth"`
	}{
		Mode:       "webrtc",
		Sdp:        sdp,
		StreamType: 0, //  1,  TRYING!!!
		Auth:       auth,
	}

	offerMqtt := &MqttMessage{
		Protocol: 302,
		Pv:       "2.2",
		T:        time.Now().Unix(),
		Data: MqttFrame{
			Header: MqttFrameHeader{
				Type:      "offer",
				From:      App.MQTTUID,
				To:        App.DeviceID,
				SubDevID:  "",
				SessionID: sessionID,
				MotoID:    motoID,
			},
			Message: offerFrame,
		},
	}

	sendBytes, err := json.Marshal(offerMqtt)
	if err != nil {
		log.Printf("marshal offer mqtt to bytes fail: %s", err.Error())

		return
	}

	publish(sendBytes)
}

func sendCandidate(sessionID string, candidate string) {
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
				From:      App.MQTTUID,
				To:        App.DeviceID,
				SubDevID:  "",
				SessionID: sessionID,
				MotoID:    motoID,
			},
			Message: candidateFrame,
		},
	}

	sendBytes, err := json.Marshal(candidateMqtt)
	if err != nil {
		log.Printf("marshal candidate mqtt to bytes fail: %s", err.Error())

		return
	}

	publish(sendBytes)
}

// 发布mqtt消息
func publish(payload []byte) {
	token := client.Publish(publishTopic, 1, false, payload)
	if token.Error() != nil {
		log.Printf("mqtt publish fail: %s, topic: %s", token.Error().Error(),
			publishTopic)
	}
}

func consume(client mqtt.Client, msg mqtt.Message,
	handleAnswerFrame func(answerFrame AnswerFrame),
	handleCandidateFrame func(candidateFrame CandidateFrame)) {
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

	dispatch(rmqtt, handleAnswerFrame, handleCandidateFrame)
}

// 分发从mqtt服务器接受到的消息
func dispatch(msg *MqttMessage,
	handleAnswerFrame func(answerFrame AnswerFrame),
	handleCandidateFrame func(candidateFrame CandidateFrame)) {

	switch msg.Data.Header.Type {
	case "answer":
		handleAnswer(msg, handleAnswerFrame)
	case "candidate":
		handleCandidate(msg, handleCandidateFrame)
	}
}

func handleAnswer(msg *MqttMessage, handleAnswerFrame func(answerFrame AnswerFrame)) {
	frame, ok := msg.Data.Message.(json.RawMessage)
	if !ok {
		log.Printf("convert interface{} to []byte fail, session: %s", msg.Data.Header.SessionID)

		return
	}

	answerFrame := AnswerFrame{}

	if err := json.Unmarshal(frame, &answerFrame); err != nil {
		log.Printf("unmarshal mqtt answer frame fail: %s, session: %s, frame: %s",
			msg.Data.Header.SessionID,
			string(msg.Data.Message.([]byte)))

		return
	}

	handleAnswerFrame(answerFrame)

}

func handleCandidate(msg *MqttMessage, handleCandidateFrame func(candidateFrame CandidateFrame)) {
	frame, ok := msg.Data.Message.(json.RawMessage)
	if !ok {
		log.Printf("convert interface{} to []byte fail, session: %s", msg.Data.Header.SessionID)

		return
	}

	candidateFrame := CandidateFrame{}

	if err := json.Unmarshal(frame, &candidateFrame); err != nil {
		log.Printf("unmarshal mqtt candidate frame fail: %s, session: %s, frame: %s",
			msg.Data.Header.SessionID,
			string(msg.Data.Message.([]byte)))

		return
	}

	// candidate from device start with "a=", end with "\r\n", which are not needed by Chrome webRTC
	candidateFrame.Candidate = strings.TrimPrefix(candidateFrame.Candidate, "a=")
	candidateFrame.Candidate = strings.TrimSuffix(candidateFrame.Candidate, "\r\n")

	handleCandidateFrame(candidateFrame)
}
