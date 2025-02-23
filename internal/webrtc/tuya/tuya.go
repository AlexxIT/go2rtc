package tuya

import (
	"errors"
	"net/url"
	"strconv"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
)

type TuyaConfig struct {
	// Set by user
	OpenAPIURL string
	ClientID   string
	Secret     string
	UID        string
	DeviceID   string

	// Set by code
	MQTTUID string
}

type tuyaSession struct {
	config          TuyaConfig
	httpAccessToken string
	sessionId       string
	mqtt            TuyaMqtt
	offerSent       core.Waiter
	connected       core.Waiter
}

func MakeTuyaSession(rawURL string, query url.Values) *tuyaSession {
	tc := &tuyaSession{}
	tc.sessionId = core.RandString(6, 62)
	tc.config.OpenAPIURL = rawURL
	tc.config.ClientID = query.Get("client_id")
	tc.config.Secret = query.Get("client_secret")
	tc.config.UID = query.Get("uid")
	tc.config.DeviceID = query.Get("device_id")
	return tc
}

var log zerolog.Logger

func TuyaClient(rawURL string, query url.Values) (core.Producer, error) {
	log = app.GetLogger("tuya")

	tc := MakeTuyaSession(rawURL, query)

	// 1. Get Tuya Auth token
	if err := tc.Authorize(); err != nil {
		return nil, err
	}

	// 2. Open Mqtt connection to device

	if err := tc.StartMqtt(); err != nil {
		return nil, err
	}

	if err := tc.mqtt.mqttReady.Wait(); err != nil {
		return nil, err
	}

	// 3. Create Peer Connection

	api, err := webrtc.NewAPI()
	if err != nil {
		return nil, err
	}

	conf := pion.Configuration{}

	conf.ICEServers, err = webrtc.UnmarshalICEServers([]byte(tc.mqtt.iceServers))
	if err != nil {
		return nil, err
	}

	pc, err := api.NewPeerConnection(conf)

	prod := webrtc.NewConn(pc)
	prod.FormatName = "webrtc/tuya"
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "ws"
	prod.URL = rawURL

	tc.mqtt.handleAnswerFrame = func(answerFrame AnswerFrame) {
		// 5. Get answer

		// HACK TO force ICERoleControlled - for some reason Tuya wants to control ICE
		desc := pion.SessionDescription{
			Type: pion.SDPTypePranswer,
			SDP:  answerFrame.Sdp,
		}
		if err = pc.SetRemoteDescription(desc); err != nil {
			return
		}
		prod.SetAnswer(answerFrame.Sdp)
		if err != nil {
			log.Error().Msgf("Failed to set answer %s", err.Error())
		}
	}
	tc.mqtt.handleCandidateFrame = func(candidateFrame CandidateFrame) {
		// 6. Continue to receiving candidates
		if candidateFrame.Candidate != "" {
			prod.AddCandidate(candidateFrame.Candidate)
			if err != nil {
				log.Error().Msgf("Failed to add candidate %s", err.Error())
			}
		}
	}

	prod.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *pion.ICECandidate:
			_ = tc.offerSent.Wait()
			tc.sendCandidate("a=" + msg.ToJSON().Candidate)

		case pion.PeerConnectionState:
			switch msg {
			case pion.PeerConnectionStateConnecting:
				break
			case pion.PeerConnectionStateConnected:
				tc.connected.Done(nil)
			default:
				tc.connected.Done(errors.New("webrtc: " + msg.String()))
			}
		}
	})

	// Order is important here, if audio comes after video, tuya sends broken SDP
	medias := []*core.Media{
		{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
	}

	// 4. Create and send offer
	offer, err := prod.CreateOffer(medias)
	if err != nil {
		return nil, err
	}

	tc.sendOffer(offer)
	tc.offerSent.Done(nil)

	// Final: Wait for connection
	if err = tc.connected.Wait(); err != nil {
		return nil, err
	}

	// Set resolution via MQTT command if set
	if query.Has("resolution_id") {
		value, err := strconv.Atoi(query.Get("resolution_id"))
		if err != nil {
			log.Error().Msgf("tuya: Failed to parse resolution_id, %s", err.Error())
			return nil, err
		}
		trySetResolution := 0
		for !tc.mqtt.resolutionSet && trySetResolution < 5 {
			trySetResolution++
			tc.sendResolution(value)
			tc.mqtt.resolutionResult.Wait()
		}
		if !tc.mqtt.resolutionSet {
			log.Warn().Msg("Failed to set resolution after 5 retries")
		}
	}

	tc.StopMqtt()

	return prod, nil
}
