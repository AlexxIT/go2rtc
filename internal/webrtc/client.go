package webrtc

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
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
				return kinesisClient(rawURL, query, "webrtc/kinesis")
			} else if format == "openipc" {
				return openIPCClient(rawURL, query)
			} else {
				return go2rtcClient(rawURL)
			}

		case "http", "https":
			if format == "milestone" {
				return milestoneClient(rawURL, query)
			} else if format == "wyze" {
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
	conn, _, err := Dial(url)
	if err != nil {
		return nil, err
	}

	// close websocket when we ready return Producer or connection error
	defer conn.Close()

	// 2. Create PeerConnection
	pc, err := PeerConnection(true)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			_ = pc.Close()
		}
	}()

	// waiter will wait PC error or WS error or nil (connection OK)
	var connState core.Waiter
	var connMu sync.Mutex

	prod := webrtc.NewConn(pc)
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "ws"
	prod.URL = url
	prod.Listen(func(msg any) {
		switch msg := msg.(type) {
		case *pion.ICECandidate:
			s := msg.ToJSON().Candidate
			log.Trace().Str("candidate", s).Msg("[webrtc] local ")
			connMu.Lock()
			_ = conn.WriteJSON(&ws.Message{Type: "webrtc/candidate", Value: s})
			connMu.Unlock()

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
	connMu.Lock()
	_ = conn.WriteJSON(msg)
	connMu.Unlock()

	// 5. Get answer
	if err = conn.ReadJSON(msg); err != nil {
		return nil, err
	}

	if msg.Type != "webrtc/answer" {
		err = errors.New("wrong answer: " + msg.String())
		return nil, err
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
	prod.Mode = core.ModeActiveProducer
	prod.Protocol = "http"
	prod.URL = url

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
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", MimeSDP)

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

// Dial - websocket.Dial with Basic auth support
func Dial(rawURL string) (*websocket.Conn, *http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, err
	}

	if u.User == nil {
		return websocket.DefaultDialer.Dial(rawURL, nil)
	}

	user := u.User.Username()
	pass, _ := u.User.Password()
	u.User = nil

	header := http.Header{
		"Authorization": []string{
			"Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)),
		},
	}

	return websocket.DefaultDialer.Dial(u.String(), header)
}
