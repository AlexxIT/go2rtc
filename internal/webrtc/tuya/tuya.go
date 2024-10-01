package tuya

import (
	"errors"
	"log"
	"net/url"
	"regexp"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v3"
)

func TuyaClient(rawURL string, query url.Values) (core.Producer, error) {
	// 1. Load config
	LoadConfig(rawURL, query)

	// 2. Get tuya auth token
	if err := InitToken(); err != nil {
		return nil, err
	}

	// 3. Get ICE servers from tuya

	_, _, iceServers, err := GetMotoIDAndAuth()
	if err != nil {
		return nil, err
	}

	// 4. Create Peer Connection

	api, err := webrtc.NewAPI()
	if err != nil {
		return nil, err
	}

	conf := pion.Configuration{}

	conf.ICEServers, err = webrtc.UnmarshalICEServers([]byte(iceServers))
	if err != nil {
		return nil, err
	}

	pc, err := api.NewPeerConnection(conf)

	// protect from sending ICE candidate before Offer
	var sendOfferWait core.Waiter

	// waiter will wait PC error or WS error or nil (connection OK)
	var connState core.Waiter

	// Tuya session id
	sessionId := core.RandString(6, 62)

	if err != nil {
		return nil, err
	}

	prod := webrtc.NewConn(pc)
	prod.FormatName = "webrtc/tuya"
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "ws"
	prod.URL = rawURL

	// 5. Open Mqtt connection for SDP and candidate exchange
	if err := StartMqtt(
		func(answerFrame AnswerFrame) {
			// 7. Get answer

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
				log.Printf("tuya: Failed to set answer %s", err.Error())
			}
		},
		func(candidateFrame CandidateFrame) {
			// 8. Continue to receiving candidates
			if candidateFrame.Candidate != "" {
				prod.AddCandidate(candidateFrame.Candidate)
				if err != nil {
					log.Printf("tuya: Failed to add candidate %s", err.Error())
				}
			}
		},
	); err != nil {
		return nil, err
	}

	// TODO: use core.Waiter to wait for mqtt ready
	time.Sleep(2 * time.Second)

	prod.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *pion.ICECandidate:
			_ = sendOfferWait.Wait()
			sendCandidate(sessionId, "a="+msg.ToJSON().Candidate)

		case pion.PeerConnectionState:
			switch msg {
			case pion.PeerConnectionStateConnecting:
			case pion.PeerConnectionStateConnected:
				connState.Done(nil)
			default:
				connState.Done(errors.New("webrtc: " + msg.String()))
			}
		}
	})

	// Order is important here, if audio comes after video, tuya sends broken SDP
	medias := []*core.Media{
		{Kind: core.KindAudio, Direction: core.DirectionSendRecv},
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
	}

	// 6. Create offer
	offer, err := prod.CreateOffer(medias)
	if err != nil {
		return nil, err
	}

	// shorter sdp, remove a=extmap... line, device ONLY allow 8KB json payload
	re := regexp.MustCompile(`\r\na=extmap[^\r\n]*`)
	offer = re.ReplaceAllString(offer, "")

	sendOffer(sessionId, offer)
	sendOfferWait.Done(nil)

	if err = connState.Wait(); err != nil {
		return nil, err
	}

	return prod, nil
}
