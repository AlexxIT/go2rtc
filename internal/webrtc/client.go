package webrtc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v3"
)

// streamsHandler supports:
//  1. WHEP:    webrtc:http://192.168.1.123:1984/api/webrtc?src=camera1
//  2. go2rtc:  webrtc:ws://192.168.1.123:1984/api/ws?src=camera1
//  3. Wyze:    webrtc:http://192.168.1.123:5000/signaling/camera1?kvs#format=wyze
//  4. Kinesis: webrtc:wss://...amazonaws.com/?...#format=kinesis#client_id=...#ice_servers=[{...},{...}]
func streamsHandler(rawURL string) (core.Producer, error) {
	var query url.Values
	if i := strings.IndexByte(rawURL, '#'); i > 0 {
		query = streams.ParseQuery(rawURL[i+1:])
		rawURL = rawURL[:i]
	}

	rawURL = rawURL[7:] // remove webrtc:
	if i := strings.IndexByte(rawURL, ':'); i > 0 {
		scheme := rawURL[:i]
		format := query.Get("format")

		switch scheme {
		case "ws", "wss":
			if format == "kinesis" {
				// https://aws.amazon.com/kinesis/video-streams/
				// https://docs.aws.amazon.com/kinesisvideostreams-webrtc-dg/latest/devguide/what-is-kvswebrtc.html
				// https://github.com/orgs/awslabs/repositories?q=kinesis+webrtc
				return kinesisClient(rawURL, query, "WebRTC/Kinesis")
			} else {
				return go2rtcClient(rawURL)
			}

		case "http", "https":
			if format == "wyze" {
				// https://github.com/mrlt8/docker-wyze-bridge
				return wyzeClient(rawURL)
			} else {
				return whepClient(rawURL)
			}
		}
	}
	return nil, errors.New("unsupported url: " + rawURL)
}

// go2rtcClient can connect only to go2rtc server
// ex: ws://localhost:1984/api/ws?src=camera1
func go2rtcClient(url string) (core.Producer, error) {
	// 1. Connect to signalign server
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}

	// close websocket when we ready return Producer or connection error
	defer conn.Close()

	// 2. Create PeerConnection
	pc, err := PeerConnection(true)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return nil, err
	}

	// waiter will wait PC error or WS error or nil (connection OK)
	var connState core.Waiter

	prod := webrtc.NewConn(pc)
	prod.Desc = "WebRTC/WebSocket async"
	prod.Mode = core.ModeActiveProducer
	prod.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *pion.ICECandidate:
			s := msg.ToJSON().Candidate
			log.Trace().Str("candidate", s).Msg("[webrtc] local")
			_ = conn.WriteJSON(&ws.Message{Type: "webrtc/candidate", Value: s})

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
		{Kind: core.KindAudio, Direction: core.DirectionSendonly},
	}

	// 3. Create offer
	offer, err := prod.CreateOffer(medias)
	if err != nil {
		return nil, err
	}

	// 4. Send offer
	msg := &ws.Message{Type: "webrtc/offer", Value: offer}
	if err = conn.WriteJSON(msg); err != nil {
		return nil, err
	}

	// 5. Get answer
	if err = conn.ReadJSON(msg); err != nil {
		return nil, err
	}

	if msg.Type != "webrtc/answer" {
		return nil, errors.New("wrong answer: " + msg.Type)
	}

	answer := msg.String()
	if err = prod.SetAnswer(answer); err != nil {
		return nil, err
	}

	// 6. Continue to receiving candidates
	go func() {
		var err error

		for {
			// receive data from remote
			var msg ws.Message
			if err = conn.ReadJSON(&msg); err != nil {
				break
			}

			switch msg.Type {
			case "webrtc/candidate":
				if msg.Value != nil {
					_ = prod.AddCandidate(msg.String())
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

// whepClient - support WebRTC-HTTP Egress Protocol (WHEP)
// ex: http://localhost:1984/api/webrtc?src=camera1
func whepClient(url string) (core.Producer, error) {
	// 2. Create PeerConnection
	pc, err := PeerConnection(true)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return nil, err
	}

	prod := webrtc.NewConn(pc)
	prod.Desc = "WebRTC/WHEP sync"
	prod.Mode = core.ModeActiveProducer

	medias := []*core.Media{
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly},
		{Kind: core.KindAudio, Direction: core.DirectionRecvonly},
	}

	// 3. Create offer
	offer, err := prod.CreateCompleteOffer(medias)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(offer))
	req.Header.Set("Content-Type", MimeSDP)
	if err != nil {
		return nil, err
	}

	client := http.Client{Timeout: time.Second * 5000}
	defer client.CloseIdleConnections()

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	answer, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if err = prod.SetAnswer(string(answer)); err != nil {
		return nil, err
	}

	return prod, nil
}

type KinesisRequest struct {
	Action   string `json:"action"`
	ClientID string `json:"recipientClientId"`
	Payload  []byte `json:"messagePayload"`
}

func (k KinesisRequest) String() string {
	return fmt.Sprintf("action=%s, payload=%s", k.Action, k.Payload)
}

type KinesisResponse struct {
	Payload []byte `json:"messagePayload"`
	Type    string `json:"messageType"`
}

func (k KinesisResponse) String() string {
	return fmt.Sprintf("type=%s, payload=%s", k.Type, k.Payload)
}

func kinesisClient(rawURL string, query url.Values, desc string) (core.Producer, error) {
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
	api, err := webrtc.NewAPI("")
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

	req := KinesisRequest{
		ClientID: query.Get("client_id"),
	}

	prod := webrtc.NewConn(pc)
	prod.Desc = desc
	prod.Mode = core.ModeActiveProducer
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
			var res KinesisResponse
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

type WyzeKVS struct {
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

	var kvs WyzeKVS
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

	return kinesisClient(kvs.URL, query, "WebRTC/Wyze")
}
