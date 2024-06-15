package webrtc

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v3"
)

func openIPCClient(rawURL string, query url.Values) (core.Producer, error) {
	// 1. Connect to signalign server
	conn, _, err := websocket.DefaultDialer.Dial(rawURL, nil)
	if err != nil {
		return nil, err
	}

	// 2. Load ICEServers from query param (base64 json)
	var conf pion.Configuration

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
	var sendAnswer core.Waiter

	// protect from blocking on errors
	defer sendAnswer.Done(nil)

	// waiter will wait PC error or WS error or nil (connection OK)
	var connState core.Waiter

	prod := webrtc.NewConn(pc)
	prod.FormatName = "webrtc/openipc"
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "ws"
	prod.URL = rawURL
	prod.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *pion.ICECandidate:
			_ = sendAnswer.Wait()

			req := openIPCReq{
				Data: msg.ToJSON().Candidate,
				Req:  "candidate",
			}
			if err = conn.WriteJSON(&req); err != nil {
				connState.Done(err)
				return
			}

			log.Trace().Msgf("[webrtc] openipc send: %s", req)

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

	go func() {
		var err error

		// will be closed when conn will be closed
		for err == nil {
			var rep openIPCReply
			if err = conn.ReadJSON(&rep); err != nil {
				// some buggy messages from Amazon servers
				if errors.Is(err, io.ErrUnexpectedEOF) {
					continue
				}
				break
			}

			log.Trace().Msgf("[webrtc] openipc recv: %s", rep)

			switch rep.Reply {
			case "webrtc_answer":
				// 6. Get answer
				var sd pion.SessionDescription
				if err = json.Unmarshal(rep.Data, &sd); err != nil {
					break
				}

				if err = prod.SetOffer(sd.SDP); err != nil {
					break
				}

				var answer string
				if answer, err = prod.GetAnswer(); err != nil {
					break
				}

				req := openIPCReq{Data: answer, Req: "answer"}
				if err = conn.WriteJSON(req); err != nil {
					break
				}

				log.Trace().Msgf("[webrtc] kinesis send: %s", req)

				sendAnswer.Done(nil)

			case "webrtc_candidate":
				// 7. Continue to receiving candidates
				var ci pion.ICECandidateInit
				if err = json.Unmarshal(rep.Data, &ci); err != nil {
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

type openIPCReply struct {
	Data  json.RawMessage `json:"data"`
	Reply string          `json:"reply"`
}

func (r openIPCReply) String() string {
	b, _ := json.Marshal(r)
	return string(b)
}

type openIPCReq struct {
	Data string `json:"data"`
	Req  string `json:"req"`
}

func (r openIPCReq) String() string {
	b, _ := json.Marshal(r)
	return string(b)
}
