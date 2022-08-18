package webrtc

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
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

	api.HandleFunc("/api/webrtc", apiHandler)
	api.HandleFunc("/api/webrtc/camera", cameraHandler)
	api.HandleWS(webrtc.MsgTypeOffer, offerHandler)
	api.HandleWS(webrtc.MsgTypeCandidate, candidateHandler)
}

func AddCandidate(address string) {
	candidates = append(candidates, address)
}

var Port string
var log zerolog.Logger
var candidates []string

var NewPConn func() (*pion.PeerConnection, error)

func apiHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	stream := streams.Get(url)
	if stream == nil {
		return
	}

	// get offer
	offer, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("[webrtc] read offer")
		return
	}

	// create new webrtc instance
	cons := new(webrtc.Conn)
	cons.Conn, err = NewPConn()
	if err != nil {
		log.Error().Err(err).Msg("[webrtc] new conn")
		return
	}

	cons.UserAgent = r.UserAgent()
	cons.Listen(func(msg interface{}) {
		if msg == streamer.StateNull {
			stream.RemoveConsumer(cons)
		}
	})

	if err = stream.AddConsumer(cons); err != nil {
		log.Warn().Err(err).Msg("[api.webrtc] add consumer")
		return
	}

	cons.Init()

	// exchange sdp with waiting all candidates
	answer, err := cons.ExchangeSDP(string(offer), true)

	// send SDP to client
	if _, err = w.Write([]byte(answer)); err != nil {
		log.Error().Err(err).Msg("[api.webrtc] send answer")
	}
}

func cameraHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	stream := streams.Get(url)
	if stream == nil {
		return
	}

	// get offer
	offer, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("[webrtc] read offer")
		return
	}

	// create new webrtc instance
	conn := new(webrtc.Conn)
	conn.Conn, err = NewPConn()
	if err != nil {
		log.Error().Err(err).Msg("[webrtc] new conn")
		return
	}

	conn.UserAgent = r.UserAgent()
	conn.Listen(func(msg interface{}) {
		switch msg.(type) {
		case pion.PeerConnectionState:
			if msg == pion.PeerConnectionStateDisconnected {
				stream.RemoveConsumer(conn)
			}
		case streamer.Track:
			//stream.AddProducer(conn)
		}
	})

	conn.Init()

	// exchange sdp with waiting all candidates
	answer, err := conn.ExchangeSDP(string(offer), true)

	// send SDP to client
	if _, err = w.Write([]byte(answer)); err != nil {
		log.Error().Err(err).Msg("[api.webrtc] send answer")
	}
}

func offerHandler(ctx *api.Context, msg *streamer.Message) {
	name := ctx.Request.URL.Query().Get("url")
	stream := streams.Get(name)
	if stream == nil {
		return
	}

	log.Debug().Str("stream", name).Msg("[webrtc] new consumer")

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
		case streamer.EventType:
			if msg == streamer.StateNull {
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
		ctx.Error(err)
		return
	}

	conn.Init()

	// exchange sdp without waiting all candidates
	//answer, err := conn.ExchangeSDP(offer, false)
	answer, err := conn.GetAnswer()
	log.Trace().Msgf("[webrtc] answer\n%s", answer)

	if err != nil {
		log.Error().Err(err).Msg("[webrtc] get answer")
		ctx.Error(err)
		return
	}

	ctx.Write(&streamer.Message{
		Type: webrtc.MsgTypeAnswer, Value: answer,
	})

	for _, address := range candidates {
		if strings.HasPrefix(address, "stun:") {
			ip, err := webrtc.GetPublicIP()
			if err != nil {
				log.Warn().Err(err).Msg("[webrtc] public IP")
				continue
			}
			address = ip.String() + address[4:]

			log.Debug().Str("addr", address).Msg("[webrtc] stun public address")
		}

		cand, err := webrtc.NewCandidate(address)
		if err != nil {
			log.Warn().Err(err).Msg("[webrtc] candidate")
			continue
		}

		conn.Fire(&streamer.Message{
			Type: webrtc.MsgTypeCandidate, Value: cand,
		})
	}

	ctx.Consumer = conn
}

func candidateHandler(ctx *api.Context, msg *streamer.Message) {
	if ctx.Consumer == nil {
		return
	}
	if conn := ctx.Consumer.(*webrtc.Conn); conn != nil {
		log.Trace().Str("candidate", msg.Value.(string)).Msg("[webrtc] remote")
		conn.Push(msg)
	}
}
