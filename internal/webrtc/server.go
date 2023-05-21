package webrtc

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v3"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const MimeSDP = "application/sdp"

var sessions = map[string]*webrtc.Conn{}

func syncHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		query := r.URL.Query()
		if query.Get("src") != "" {
			// WHEP or JSON SDP or raw SDP exchange
			outputWebRTC(w, r)
		} else if query.Get("dst") != "" {
			// WHIP SDP exchange
			inputWebRTC(w, r)
		} else {
			http.Error(w, "", http.StatusBadRequest)
		}

	case "PATCH":
		// TODO: WHEP/WHIP
		http.Error(w, "", http.StatusMethodNotAllowed)

	case "DELETE":
		if id := r.URL.Query().Get("id"); id != "" {
			if conn, ok := sessions[id]; ok {
				delete(sessions, id)
				_ = conn.Close()
			} else {
				http.Error(w, "", http.StatusNotFound)
			}
		} else {
			http.Error(w, "", http.StatusBadRequest)
		}

	default:
		http.Error(w, "", http.StatusMethodNotAllowed)
	}
}

// outputWebRTC support API depending on Content-Type:
// 1. application/json - receive {"type":"offer","sdp":"v=0\r\n..."} and response {"type":"answer","sdp":"v=0\r\n..."}
// 2. application/sdp - receive/response SDP via WebRTC-HTTP Egress Protocol (WHEP)
// 3. other - receive/response raw SDP
func outputWebRTC(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("src")
	stream := streams.Get(url)
	if stream == nil {
		return
	}

	mediaType := r.Header.Get("Content-Type")
	if mediaType != "" {
		mediaType, _, _ = strings.Cut(mediaType, ";")
		mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	}

	var offer string

	switch mediaType {
	case "application/json":
		var desc pion.SessionDescription
		if err := json.NewDecoder(r.Body).Decode(&desc); err != nil {
			log.Error().Err(err).Caller().Send()
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		offer = desc.SDP

	default:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Error().Err(err).Caller().Send()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		offer = string(body)
	}

	var desc string

	switch mediaType {
	case "application/json":
		desc = "WebRTC/JSON sync"
	case MimeSDP:
		desc = "WebRTC/WHEP sync"
	default:
		desc = "WebRTC/HTTP sync"
	}

	answer, err := ExchangeSDP(stream, offer, desc, r.UserAgent())
	if err != nil {
		log.Error().Err(err).Caller().Send()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch mediaType {
	case "application/json":
		w.Header().Set("Content-Type", mediaType)

		v := pion.SessionDescription{
			Type: pion.SDPTypeAnswer, SDP: answer,
		}
		err = json.NewEncoder(w).Encode(v)

	case MimeSDP:
		w.Header().Set("Content-Type", mediaType)
		w.WriteHeader(http.StatusCreated)

		_, err = w.Write([]byte(answer))

	default:
		_, err = w.Write([]byte(answer))
	}

	if err != nil {
		log.Error().Err(err).Caller().Send()
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func inputWebRTC(w http.ResponseWriter, r *http.Request) {
	dst := r.URL.Query().Get("dst")
	stream := streams.Get(dst)
	if stream == nil {
		stream = streams.New(dst, nil)
	}

	// 1. Get offer
	offer, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Trace().Msgf("[webrtc] WHIP offer\n%s", offer)

	pc, err := PeerConnection(false)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// create new webrtc instance
	prod := webrtc.NewConn(pc)
	prod.Desc = "WebRTC/WHIP sync"
	prod.Mode = core.ModePassiveProducer
	prod.UserAgent = r.UserAgent()

	if err = prod.SetOffer(string(offer)); err != nil {
		log.Warn().Err(err).Caller().Send()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	answer, err := prod.GetCompleteAnswer()
	if err == nil {
		answer, err = syncCanditates(answer)
	}
	if err != nil {
		log.Warn().Err(err).Caller().Send()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Trace().Msgf("[webrtc] WHIP answer\n%s", answer)

	id := strconv.FormatInt(time.Now().UnixNano(), 36)
	sessions[id] = prod

	prod.Listen(func(msg any) {
		switch msg := msg.(type) {
		case pion.PeerConnectionState:
			if msg == pion.PeerConnectionStateClosed {
				stream.RemoveProducer(prod)
				if _, ok := sessions[id]; ok {
					delete(sessions, id)
				}
			}
		}
	})

	stream.AddProducer(prod)

	w.Header().Set("Content-Type", MimeSDP)
	w.Header().Set("Location", "webrtc?id="+id)
	w.WriteHeader(http.StatusCreated)

	if _, err = w.Write([]byte(answer)); err != nil {
		log.Warn().Err(err).Caller().Send()
		return
	}
}
