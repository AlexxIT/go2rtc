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

	LoadConfig(rawURL, query)

	if err := InitToken(); err != nil {
		return nil, err
	}

	api, err := webrtc.NewAPI()
	if err != nil {
		return nil, err
	}

	_, _, iceServers, err = GetMotoIDAndAuth()
	if err != nil {
		return nil, err
	}

	// protect from sending ICE candidate before Offer
	var sendOfferWait core.Waiter

	// waiter will wait PC error or WS error or nil (connection OK)
	var connState core.Waiter

	sessionId := core.RandString(6, 62)

	conf := pion.Configuration{}

	conf.ICEServers, err = webrtc.UnmarshalICEServers([]byte(iceServers))
	if err != nil {
		return nil, err
	}

	pc, err := api.NewPeerConnection(conf)

	if err != nil {
		return nil, err
	}

	prod := webrtc.NewConn(pc)
	prod.FormatName = "webrtc/tuya"
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "ws"
	prod.URL = rawURL

	if err := StartMqtt(
		func(answerFrame AnswerFrame) {
			role := pion.ICERoleControlled
			pc.ForceIceRole = &role
			err = prod.SetAnswer(answerFrame.Sdp)
			if err != nil {
				log.Printf("tuya: Failed to set answer %s", err.Error())
			}
		},
		func(candidateFrame CandidateFrame) {
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

	medias := []*core.Media{
		{Kind: core.KindAudio, Direction: core.DirectionSendRecv},
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
	}

	// 4. Create offer
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
