package webrtc

import (
	"encoding/json"
	"errors"
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
	"io"
	"mime"
	"net"
	"net/http"
)

type jsonSdp struct {
	Type string `json:"type"`
	Sdp  string `json:"sdp"`
}

func Init() {
	var cfg struct {
		Mod struct {
			Listen     string           `yaml:"listen"`
			Candidates []string         `yaml:"candidates"`
			IceServers []pion.ICEServer `yaml:"ice_servers"`
		} `yaml:"webrtc"`
	}

	cfg.Mod.Listen = ":8555"
	cfg.Mod.IceServers = []pion.ICEServer{
		{URLs: []string{"stun:stun.l.google.com:19302"}},
	}

	app.LoadConfig(&cfg)

	log = app.GetLogger("webrtc")

	address := cfg.Mod.Listen
	pionAPI, err := webrtc.NewAPI(address)
	if pionAPI == nil {
		log.Error().Err(err).Caller().Msg("webrtc.NewAPI")
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

	api.HandleWS("webrtc/offer", asyncHandler)
	api.HandleWS("webrtc/candidate", candidateHandler)

	api.HandleFunc("api/webrtc", syncHandler)
}

var Port string
var log zerolog.Logger

var NewPConn func() (*pion.PeerConnection, error)

func asyncHandler(tr *api.Transport, msg *api.Message) error {
	src := tr.Request.URL.Query().Get("src")
	stream := streams.GetOrNew(src)
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	log.Debug().Str("url", src).Msg("[webrtc] new consumer")

	var err error

	// create new webrtc instance
	conn := new(webrtc.Conn)
	conn.Conn, err = NewPConn()
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return err
	}

	conn.UserAgent = tr.Request.UserAgent()
	conn.Listen(func(msg interface{}) {
		switch msg := msg.(type) {
		case pion.PeerConnectionState:
			if msg == pion.PeerConnectionStateClosed {
				stream.RemoveConsumer(conn)
			}
		case *pion.ICECandidate:
			if msg != nil {
				s := msg.ToJSON().Candidate
				log.Trace().Str("candidate", s).Msg("[webrtc] local")
				tr.Write(&api.Message{Type: "webrtc/candidate", Value: s})
			}
		}
	})

	// 1. SetOffer, so we can get remote client codecs
	offer := msg.Value.(string)
	log.Trace().Msgf("[webrtc] offer:\n%s", offer)

	if err = conn.SetOffer(offer); err != nil {
		log.Warn().Err(err).Caller().Send()
		return err
	}

	// 2. AddConsumer, so we get new tracks
	if err = stream.AddConsumer(conn); err != nil {
		log.Debug().Err(err).Msg("[webrtc] add consumer")
		_ = conn.Conn.Close()
		return err
	}

	conn.Init()

	// 3. Exchange SDP without waiting all candidates
	answer, err := conn.GetAnswer()
	log.Trace().Msgf("[webrtc] answer\n%s", answer)

	if err != nil {
		log.Error().Err(err).Caller().Send()
		return err
	}

	tr.Consumer = conn

	tr.Write(&api.Message{Type: "webrtc/answer", Value: answer})

	asyncCandidates(tr)

	return nil
}

func syncHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("src")
	stream := streams.Get(url)
	if stream == nil {
		return
	}

	mediaType, err := DetermineMediaType(r)
	if err != nil {
		log.Error().Err(err).Caller().Msg("DetermineMediaType")
		return
	}

	// get offer
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Caller().Msg("ioutil.ReadAll")
		return
	}

	offer, err := DeserializeOffer(mediaType, body)
	if err != nil {
		log.Error().Err(err).Caller().Msg("DeserializeOffer")
		return
	}

	answer, err := ExchangeSDP(stream, offer, r.UserAgent())
	if err != nil {
		log.Error().Err(err).Caller().Msg("ExchangeSDP")
		return
	}

	response, err := SerializeAnswer(mediaType, answer)
	if err != nil {
		log.Error().Err(err).Caller().Msg("SerializeAnswer")
		return
	}

	// send SDP to client
	if _, err = w.Write(response); err != nil {
		log.Error().Err(err).Caller().Msg("w.Write")
	}
}

func DetermineMediaType(r *http.Request) (mediaType string, err error) {
	contentType := r.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err = mime.ParseMediaType(contentType)
		if err != nil {
			log.Error().Err(err).Caller().Msg("mime.ParseMediaType")
			return
		}
	} else {
		mediaType = "text/plain"
	}
	return
}

func DeserializeOffer(mediaType string, body []byte) (offer string, err error) {
	switch mediaType {
	case "application/json":
		var jsonOffer jsonSdp
		err = json.Unmarshal(body, &jsonOffer)
		if err != nil {
			log.Error().Err(err).Caller().Msg("json.Unmarshal")
			return
		}

		offer = jsonOffer.Sdp
		break
	default:
		offer = string(body)
		break
	}
	return
}

func ExchangeSDP(
	stream *streams.Stream, offer string, userAgent string,
) (answer string, err error) {
	// create new webrtc instance
	conn := new(webrtc.Conn)
	conn.Conn, err = NewPConn()
	if err != nil {
		log.Error().Err(err).Caller().Msg("NewPConn")
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
		log.Warn().Err(err).Caller().Msg("conn.SetOffer")
		return
	}

	// 2. AddConsumer, so we get new tracks
	if err = stream.AddConsumer(conn); err != nil {
		log.Warn().Err(err).Caller().Msg("stream.AddConsumer")
		_ = conn.Conn.Close()
		return
	}

	conn.Init()

	// exchange sdp without waiting all candidates
	//answer, err := conn.ExchangeSDP(offer, false)
	answer, err = conn.GetCompleteAnswer()
	if err == nil {
		answer, err = syncCanditates(answer)
	}
	log.Trace().Msgf("[webrtc] answer\n%s", answer)

	if err != nil {
		log.Error().Err(err).Caller().Msg("conn.GetCompleteAnswer")
	}

	return
}

func SerializeAnswer(mediaType string, answer string) (response []byte, err error) {
	switch mediaType {
	case "application/json":
		jsonAnswer := jsonSdp{
			Sdp:  answer,
			Type: "answer",
		}
		response, err = json.Marshal(jsonAnswer)
		if err != nil {
			log.Error().Err(err).Caller().Msg("json.Marshal")
			return
		}
		break
	default:
		response = []byte(answer)
		break
	}
	return
}
