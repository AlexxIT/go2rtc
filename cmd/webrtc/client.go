package webrtc

import (
	"errors"
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v3"
	"io"
	"net/http"
	"strings"
	"time"
)

func streamsHandler(url string) (streamer.Producer, error) {
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

func asyncClient(url string) (streamer.Producer, error) {
	// 1. Connect to signalign server
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = ws.Close()
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
	prod.Listen(func(msg any) {
		switch msg := msg.(type) {
		case pion.PeerConnectionState:
			_ = ws.Close()

		case *pion.ICECandidate:
			sendOffer.Wait()

			s := msg.ToJSON().Candidate
			log.Trace().Str("candidate", s).Msg("[webrtc] local")
			_ = ws.WriteJSON(&api.Message{Type: "webrtc/candidate", Value: s})
		}
	})

	medias := []*streamer.Media{
		{Kind: streamer.KindVideo, Direction: streamer.DirectionRecvonly},
		{Kind: streamer.KindAudio, Direction: streamer.DirectionRecvonly},
	}

	// 3. Create offer
	offer, err := prod.CreateOffer(medias)
	if err != nil {
		return nil, err
	}

	// 4. Send offer
	msg := &api.Message{Type: "webrtc/offer", Value: offer}
	if err = ws.WriteJSON(msg); err != nil {
		return nil, err
	}

	sendOffer.Done()

	// 5. Get answer
	if err = ws.ReadJSON(msg); err != nil {
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
			msg := new(api.Message)
			if err = ws.ReadJSON(msg); err != nil {
				if cerr, ok := err.(*websocket.CloseError); ok {
					log.Trace().Err(err).Caller().Msgf("[webrtc] ws code=%d", cerr)
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

		_ = ws.Close()
	}()

	return prod, nil
}

// syncClient - support WebRTC-HTTP Egress Protocol (WHEP)
func syncClient(url string) (streamer.Producer, error) {
	// 2. Create PeerConnection
	pc, err := PeerConnection(true)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return nil, err
	}

	prod := webrtc.NewConn(pc)

	medias := []*streamer.Media{
		{Kind: streamer.KindVideo, Direction: streamer.DirectionRecvonly},
		{Kind: streamer.KindAudio, Direction: streamer.DirectionRecvonly},
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
