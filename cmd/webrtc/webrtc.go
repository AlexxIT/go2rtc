package webrtc

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
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

	cfg.Mod.IceServers = []pion.ICEServer{
		{URLs: []string{"stun:stun.l.google.com:19302"}},
	}

	app.LoadConfig(&cfg)

	log = app.GetLogger("webrtc")

	address := cfg.Mod.Listen
	pionAPI, err := webrtc.NewAPI(address)
	if pionAPI == nil {
		log.Error().Err(err).Msg("[webrtc] init API")
		return
	}

	if err != nil {
		log.Warn().Err(err).Msg("[webrtc] listen")
	} else if address != "" {
		log.Info().Str("addr", address).Msg("[webrtc] listen")
		_, Port, _ = net.SplitHostPort(address)
	}

	pionConf := pion.Configuration{
		ICEServers:   cfg.Mod.IceServers,
		SDPSemantics: pion.SDPSemanticsUnifiedPlanWithFallback,
	}

	NewPConn = func() (*pion.PeerConnection, error) {
		return pionAPI.NewPeerConnection(pionConf)
	}

	candidates = cfg.Mod.Candidates

	api.HandleWS(webrtc.MsgTypeOffer, offerHandler)
	api.HandleWS(webrtc.MsgTypeCandidate, candidateHandler)
}

var Port string
var log zerolog.Logger

var NewPConn func() (*pion.PeerConnection, error)

func offerHandler(ctx *api.Context, msg *streamer.Message) {
	src := ctx.Request.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		return
	}

	log.Debug().Str("src", src).Msg("[webrtc] new consumer")

	var err error

	// create new webrtc instance
	conn := new(webrtc.Conn)
	conn.Conn, err = NewPConn()
	if err != nil {
		log.Error().Err(err).Msg("[webrtc] new conn")
		return
	}

	conn.UserAgent = ctx.Request.UserAgent()
	conn.Listen(func(msg interface{}) {
		switch msg := msg.(type) {
		case pion.PeerConnectionState:
			if msg == pion.PeerConnectionStateClosed {
				stream.RemoveConsumer(conn)
			}
		case *streamer.Message:
			// subscribe on webrtc server candidates
			log.Trace().Str("candidate", msg.Value.(string)).Msg("[webrtc] local")
			ctx.Write(msg)
		}
	})

	// 1. SetOffer, so we can get remote client codecs
	offer := msg.Value.(string)
	log.Trace().Msgf("[webrtc] offer:\n%s", offer)

	if err = conn.SetOffer(offer); err != nil {
		log.Warn().Err(err).Msg("[api.webrtc] set offer")
		ctx.Error(err)
		return
	}

	// 2. AddConsumer, so we get new tracks
	if err = stream.AddConsumer(conn); err != nil {
		log.Warn().Err(err).Msg("[api.webrtc] add consumer")
		_ = conn.Conn.Close()
		ctx.Error(err)
		return
	}

	conn.Init()

	// exchange sdp without waiting all candidates
	//answer, err := conn.ExchangeSDP(offer, false)
	//answer, err := conn.GetAnswer()
	answer, err := conn.GetCompleteAnswer()
	if err == nil {
		answer, err = addCanditates(answer)
	}
	log.Trace().Msgf("[webrtc] answer\n%s", answer)

	if err != nil {
		log.Error().Err(err).Msg("[webrtc] get answer")
		ctx.Error(err)
		return
	}

	ctx.Write(&streamer.Message{
		Type: webrtc.MsgTypeAnswer, Value: answer,
	})

	ctx.Consumer = conn
}

func ExchangeSDP(
	stream *streams.Stream, offer string, userAgent string,
) (answer string, err error) {
	// create new webrtc instance
	conn := new(webrtc.Conn)
	conn.Conn, err = NewPConn()
	if err != nil {
		log.Error().Err(err).Msg("[webrtc] new conn")
		return
	}

	conn.UserAgent = userAgent
	conn.Listen(func(msg interface{}) {
		switch msg := msg.(type) {
		case pion.PeerConnectionState:
			if msg == pion.PeerConnectionStateClosed {
				stream.RemoveConsumer(conn)
			}
		}
	})

	// 1. SetOffer, so we can get remote client codecs
	log.Trace().Msgf("[webrtc] offer:\n%s", offer)

	if err = conn.SetOffer(offer); err != nil {
		log.Warn().Err(err).Msg("[api.webrtc] set offer")
		return
	}

	// 2. AddConsumer, so we get new tracks
	if err = stream.AddConsumer(conn); err != nil {
		log.Warn().Err(err).Msg("[api.webrtc] add consumer")
		_ = conn.Conn.Close()
		return
	}

	conn.Init()

	// exchange sdp without waiting all candidates
	//answer, err := conn.ExchangeSDP(offer, false)
	answer, err = conn.GetCompleteAnswer()
	if err == nil {
		answer, err = addCanditates(answer)
	}
	log.Trace().Msgf("[webrtc] answer\n%s", answer)

	if err != nil {
		log.Error().Err(err).Msg("[webrtc] get answer")
	}

	return
}
