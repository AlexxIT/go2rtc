package webrtc

import (
	"errors"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v3"
	"io"
	"net/http"
	"strings"
	"time"
)

func streamsHandler(url string) (core.Producer, error) {
	url = url[7:]
	if i := strings.Index(url, "://"); i > 0 {
		switch url[:i] {
		case "ws", "wss":
			return asyncClient(url)
		case "http", "https":
			return syncClient(url)
		}
	}
	return nil, errors.New("unsupported url: " + url)
}

// asyncClient can connect only to go2rtc server
// ex: ws://localhost:1984/api/ws?src=camera1
func asyncClient(url string) (core.Producer, error) {
	// 1. Connect to signalign server
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()

	// 2. Create PeerConnection
	pc, err := PeerConnection(true)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return nil, err
	}

	var sendOffer core.Waiter

	prod := webrtc.NewConn(pc)
	prod.Desc = "WebRTC/WebSocket async"
	prod.Mode = core.ModeActiveProducer
	prod.Listen(func(msg any) {
		switch msg := msg.(type) {
		case pion.PeerConnectionState:
			_ = conn.Close()

		case *pion.ICECandidate:
			sendOffer.Wait()

			s := msg.ToJSON().Candidate
			log.Trace().Str("candidate", s).Msg("[webrtc] local")
			_ = conn.WriteJSON(&ws.Message{Type: "webrtc/candidate", Value: s})
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

	sendOffer.Done()

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
		for {
			// receive data from remote
			msg := new(ws.Message)
			if err = conn.ReadJSON(msg); err != nil {
				if cerr, ok := err.(*websocket.CloseError); ok {
					log.Trace().Err(err).Caller().Msgf("[webrtc] ws code=%d", cerr.Code)
				}
				break
			}

			switch msg.Type {
			case "webrtc/candidate":
				if msg.Value != nil {
					_ = prod.AddCandidate(msg.String())
				}
			}
		}

		_ = conn.Close()
	}()

	return prod, nil
}

// syncClient - support WebRTC-HTTP Egress Protocol (WHEP)
// ex: http://localhost:1984/api/webrtc?src=camera1
func syncClient(url string) (core.Producer, error) {
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
