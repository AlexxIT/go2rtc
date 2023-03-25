package webrtc

import (
	"errors"
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
	"net"
)

func Init() {
	var cfg struct {
		Mod struct {
			Listen     string           `yaml:"listen"`
			Candidates []string         `yaml:"candidates"`
			IceServers []pion.ICEServer `yaml:"ice_servers"`
		} `yaml:"webrtc"`
	}

	cfg.Mod.Listen = ":8555/tcp"
	cfg.Mod.IceServers = []pion.ICEServer{
		{URLs: []string{"stun:stun.l.google.com:19302"}},
	}

	app.LoadConfig(&cfg)

	log = app.GetLogger("webrtc")

	address := cfg.Mod.Listen

	// create pionAPI with custom codecs list and custom network settings
	serverAPI, err := webrtc.NewAPI(address)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	// use same API for WebRTC server and client if no address
	clientAPI := serverAPI

	if address != "" {
		log.Info().Str("addr", address).Msg("[webrtc] listen")
		_, Port, _ = net.SplitHostPort(address)

		clientAPI, _ = webrtc.NewAPI("")
	}

	pionConf := pion.Configuration{
		ICEServers:   cfg.Mod.IceServers,
		SDPSemantics: pion.SDPSemanticsUnifiedPlanWithFallback,
	}

	PeerConnection = func(active bool) (*pion.PeerConnection, error) {
		// active - client, passive - server
		if active {
			return clientAPI.NewPeerConnection(pionConf)
		} else {
			return serverAPI.NewPeerConnection(pionConf)
		}
	}

	for _, candidate := range cfg.Mod.Candidates {
		AddCandidate(candidate)
	}

	// async WebRTC server (two API versions)
	api.HandleWS("webrtc", asyncHandler)
	api.HandleWS("webrtc/offer", asyncHandler)
	api.HandleWS("webrtc/candidate", candidateHandler)

	// sync WebRTC server (two API versions)
	api.HandleFunc("api/webrtc", syncHandler)

	// WebRTC client
	streams.HandleFunc("webrtc", streamsHandler)
}

var Port string
var log zerolog.Logger

var PeerConnection func(active bool) (*pion.PeerConnection, error)

func asyncHandler(tr *api.Transport, msg *api.Message) error {
	var stream *streams.Stream
	var mode core.Mode

	query := tr.Request.URL.Query()
	if name := query.Get("src"); name != "" {
		stream = streams.GetOrNew(name)
		mode = core.ModePassiveConsumer
		log.Debug().Str("src", name).Msg("[webrtc] new consumer")
	} else if name = query.Get("dst"); name != "" {
		stream = streams.Get(name)
		mode = core.ModePassiveProducer
		log.Debug().Str("src", name).Msg("[webrtc] new producer")
	}

	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	// create new PeerConnection instance
	pc, err := PeerConnection(false)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return err
	}

	var sendAnswer core.Waiter

	conn := webrtc.NewConn(pc)
	conn.Desc = "WebRTC/WebSocket async"
	conn.Mode = mode
	conn.UserAgent = tr.Request.UserAgent()
	conn.Listen(func(msg any) {
		switch msg := msg.(type) {
		case pion.PeerConnectionState:
			if msg != pion.PeerConnectionStateClosed {
				return
			}
			switch mode {
			case core.ModePassiveConsumer:
				stream.RemoveConsumer(conn)
			case core.ModePassiveProducer:
				stream.RemoveProducer(conn)
			}

		case *pion.ICECandidate:
			sendAnswer.Wait()

			s := msg.ToJSON().Candidate
			log.Trace().Str("candidate", s).Msg("[webrtc] local")
			tr.Write(&api.Message{Type: "webrtc/candidate", Value: s})
		}
	})

	// V2 - json/object exchange, V1 - raw SDP exchange
	apiV2 := msg.Type == "webrtc"

	// 1. SetOffer, so we can get remote client codecs
	var offer string
	if apiV2 {
		offer = msg.GetString("sdp")
	} else {
		offer = msg.String()
	}

	log.Trace().Msgf("[webrtc] offer:\n%s", offer)

	if err = conn.SetOffer(offer); err != nil {
		log.Warn().Err(err).Caller().Send()
		return err
	}

	switch mode {
	case core.ModePassiveConsumer:
		// 2. AddConsumer, so we get new tracks
		if err = stream.AddConsumer(conn); err != nil {
			log.Debug().Err(err).Msg("[webrtc] add consumer")
			_ = conn.Close()
			return err
		}
	case core.ModePassiveProducer:
		stream.AddProducer(conn)
	}

	// 3. Exchange SDP without waiting all candidates
	answer, err := conn.GetAnswer()
	log.Trace().Msgf("[webrtc] answer\n%s", answer)

	if err != nil {
		log.Error().Err(err).Caller().Send()
		return err
	}

	if apiV2 {
		desc := pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: answer}
		tr.Write(&api.Message{Type: "webrtc", Value: desc})
	} else {
		tr.Write(&api.Message{Type: "webrtc/answer", Value: answer})
	}

	sendAnswer.Done()

	asyncCandidates(tr, conn)

	return nil
}

func ExchangeSDP(stream *streams.Stream, offer, desc, userAgent string) (answer string, err error) {
	pc, err := PeerConnection(false)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	// create new webrtc instance
	conn := webrtc.NewConn(pc)
	conn.Desc = desc
	conn.UserAgent = userAgent
	conn.Listen(func(msg any) {
		switch msg := msg.(type) {
		case pion.PeerConnectionState:
			if msg != pion.PeerConnectionStateClosed {
				return
			}
			if conn.Mode == core.ModePassiveConsumer {
				stream.RemoveConsumer(conn)
			} else {
				stream.RemoveProducer(conn)
			}
		}
	})

	// 1. SetOffer, so we can get remote client codecs
	log.Trace().Msgf("[webrtc] offer:\n%s", offer)

	if err = conn.SetOffer(offer); err != nil {
		log.Warn().Err(err).Caller().Send()
		return
	}

	if IsConsumer(conn) {
		conn.Mode = core.ModePassiveConsumer

		// 2. AddConsumer, so we get new tracks
		if err = stream.AddConsumer(conn); err != nil {
			log.Warn().Err(err).Caller().Send()
			_ = conn.Close()
			return
		}
	} else {
		conn.Mode = core.ModePassiveProducer

		stream.AddProducer(conn)
	}

	answer, err = conn.GetCompleteAnswer()
	if err == nil {
		answer, err = syncCanditates(answer)
	}
	log.Trace().Msgf("[webrtc] answer\n%s", answer)

	if err != nil {
		log.Error().Err(err).Caller().Send()
	}

	return
}

func IsConsumer(conn *webrtc.Conn) bool {
	// if wants get video - consumer
	for _, media := range conn.GetMedias() {
		if media.Kind == core.KindVideo && media.Direction == core.DirectionSendonly {
			return true
		}
	}
	// if wants send video - producer
	for _, media := range conn.GetMedias() {
		if media.Kind == core.KindVideo && media.Direction == core.DirectionRecvonly {
			return false
		}
	}
	// if wants something - consumer
	for _, media := range conn.GetMedias() {
		if media.Direction == core.DirectionSendonly {
			return true
		}
	}
	return false
}
