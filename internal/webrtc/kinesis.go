package webrtc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v3"
)

type kinesisRequest struct {
	Action   string `json:"action"`
	ClientID string `json:"recipientClientId"`
	Payload  []byte `json:"messagePayload"`
}

func (k kinesisRequest) String() string {
	return fmt.Sprintf("action=%s, payload=%s", k.Action, k.Payload)
}

type kinesisResponse struct {
	Payload []byte `json:"messagePayload"`
	Type    string `json:"messageType"`
}

func (k kinesisResponse) String() string {
	return fmt.Sprintf("type=%s, payload=%s", k.Type, k.Payload)
}

func kinesisClient(rawURL string, query url.Values, format string) (core.Producer, error) {
	// 1. Connect to signalign server
	conn, _, err := websocket.DefaultDialer.Dial(rawURL, nil)
	if err != nil {
		return nil, err
	}

	// 2. Load ICEServers from query param (base64 json)
	conf := pion.Configuration{}

	if s := query.Get("ice_servers"); s != "" {
		conf.ICEServers, err = webrtc.UnmarshalICEServers([]byte(s))
		if err != nil {
			log.Warn().Err(err).Caller().Send()
		}
	}

	// close websocket when we ready return Producer or connection error
	defer conn.Close()

	// 3. Create Peer Connection
	api, err := webrtc.NewAPI()
	if err != nil {
		return nil, err
	}

	pc, err := api.NewPeerConnection(conf)
	if err != nil {
		return nil, err
	}

	// protect from sending ICE candidate before Offer
	var sendOffer core.Waiter

	// protect from blocking on errors
	defer sendOffer.Done(nil)

	// waiter will wait PC error or WS error or nil (connection OK)
	var connState core.Waiter

	req := kinesisRequest{
		ClientID: query.Get("client_id"),
	}

	prod := webrtc.NewConn(pc)
	prod.FormatName = format
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "ws"
	prod.URL = rawURL
	prod.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *pion.ICECandidate:
			_ = sendOffer.Wait()

			req.Action = "ICE_CANDIDATE"
			req.Payload, _ = json.Marshal(msg.ToJSON())
			if err = conn.WriteJSON(&req); err != nil {
				connState.Done(err)
				return
			}

			log.Trace().Msgf("[webrtc] kinesis send: %s", req)

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
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
		{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
	}

	// 4. Create offer
	offer, err := prod.CreateOffer(medias)
	if err != nil {
		return nil, err
	}

	// 5. Send offer
	req.Action = "SDP_OFFER"
	req.Payload, _ = json.Marshal(pion.SessionDescription{
		Type: pion.SDPTypeOffer,
		SDP:  offer,
	})
	if err = conn.WriteJSON(req); err != nil {
		return nil, err
	}

	log.Trace().Msgf("[webrtc] kinesis send: %s", req)

	sendOffer.Done(nil)

	go func() {
		var err error

		// will be closed when conn will be closed
		for {
			var res kinesisResponse
			if err = conn.ReadJSON(&res); err != nil {
				// some buggy messages from Amazon servers
				if errors.Is(err, io.ErrUnexpectedEOF) {
					continue
				}
				break
			}

			log.Trace().Msgf("[webrtc] kinesis recv: %s", res)

			switch res.Type {
			case "SDP_ANSWER":
				// 6. Get answer
				var sd pion.SessionDescription
				if err = json.Unmarshal(res.Payload, &sd); err != nil {
					break
				}

				if err = prod.SetAnswer(sd.SDP); err != nil {
					break
				}

			case "ICE_CANDIDATE":
				// 7. Continue to receiving candidates
				var ci pion.ICECandidateInit
				if err = json.Unmarshal(res.Payload, &ci); err != nil {
					break
				}

				if err = prod.AddCandidate(ci.Candidate); err != nil {
					break
				}
			}
		}

		connState.Done(err)
	}()

	if err = connState.Wait(); err != nil {
		return nil, err
	}

	return prod, nil
}

type wyzeKVS struct {
	ClientId string          `json:"ClientId"`
	Cam      string          `json:"cam"`
	Result   string          `json:"result"`
	Servers  json.RawMessage `json:"servers"`
	URL      string          `json:"signalingUrl"`
}

func wyzeClient(rawURL string) (core.Producer, error) {
	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Get(rawURL)
	if err != nil {
		return nil, err
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var kvs wyzeKVS
	if err = json.Unmarshal(b, &kvs); err != nil {
		return nil, err
	}

	if kvs.Result != "ok" {
		return nil, errors.New("wyse: wrong result: " + kvs.Result)
	}

	query := url.Values{
		"client_id":   []string{kvs.ClientId},
		"ice_servers": []string{string(kvs.Servers)},
	}

	return kinesisClient(kvs.URL, query, "webrtc/wyze")
}
