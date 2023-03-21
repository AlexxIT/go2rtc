package webtorrent

import (
	"encoding/base64"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/gorilla/websocket"
	"sync"
	"time"
)

type Server struct {
	core.Listener

	URL      string
	Exchange func(src, offer string) (answer string, err error)

	shares   map[string]*Share
	mu       sync.Mutex
	announce *core.Worker
}

type Share struct {
	name string
	pwd  string
	src  string
}

func (s *Server) AddShare(name, pwd, src string) {
	s.mu.Lock()

	if s.shares == nil {
		s.shares = map[string]*Share{}
	}

	if len(s.shares) == 0 {
		go s.Serve()
	}

	hash := InfoHash(name)
	s.shares[hash] = &Share{
		name: name,
		pwd:  pwd,
		src:  src,
	}

	s.announce.Do()

	s.mu.Unlock()
}

func (s *Server) GetSharePwd(name string) (pwd string) {
	hash := InfoHash(name)
	s.mu.Lock()
	if share, ok := s.shares[hash]; ok {
		pwd = share.pwd
	}
	s.mu.Unlock()
	return
}

func (s *Server) RemoveShare(name string) {
	hash := InfoHash(name)
	s.mu.Lock()
	if _, ok := s.shares[hash]; ok {
		delete(s.shares, hash)
	}
	s.mu.Unlock()
}

// Serve - run reconnection loop, will exit on??
func (s *Server) Serve() error {
	for s.HasShares() {
		s.Fire("connect to tracker: " + s.URL)

		ws, _, err := websocket.DefaultDialer.Dial(s.URL, nil)
		if err != nil {
			s.Fire(err)
			time.Sleep(time.Minute)
			continue
		}

		peerID := core.RandString(16, 36)

		// instant run announce worker
		s.announce = core.NewWorker(0, func() time.Duration {
			if err = s.writer(ws, peerID); err != nil {
				return 0
			}
			return time.Minute
		})

		// run reader forewer
		for {
			if err = s.reader(ws, peerID); err != nil {
				break
			}
		}

		// stop announcing worker
		s.announce.Stop()

		// ensure ws is stopped
		_ = ws.Close()

		time.Sleep(time.Minute)
	}

	s.Fire("disconnect")

	return nil
}

// reader - receive offers in the loop, will exit on ws.Close
func (s *Server) reader(ws *websocket.Conn, peerID string) error {
	var v Message
	if err := ws.ReadJSON(&v); err != nil {
		return err
	}

	s.Fire(&v)

	s.mu.Lock()
	share, ok := s.shares[v.InfoHash]
	s.mu.Unlock()

	// skip any unknown shares
	if !ok || v.OfferId == "" || v.Offer == nil {
		return nil
	}

	s.Fire("new offer: " + v.OfferId)

	cipher, err := NewCipher(share.name, share.pwd, v.OfferId)
	if err != nil {
		s.Fire(err)
		return nil
	}

	enc, err := base64.StdEncoding.DecodeString(v.Offer.SDP)
	if err != nil {
		s.Fire(err)
		return nil
	}

	offer, err := cipher.Decrypt(enc)
	if err != nil {
		s.Fire(err)
		return nil
	}

	answer, err := s.Exchange(share.src, string(offer))
	if err != nil {
		s.Fire(err)
		return nil
	}

	enc = cipher.Encrypt([]byte(answer))

	raw := fmt.Sprintf(
		`{"action":"announce","info_hash":"%s","peer_id":"%s","offer_id":"%s","answer":{"type":"answer","sdp":"%s"},"to_peer_id":"%s"}`,
		v.InfoHash, peerID, v.OfferId, base64.StdEncoding.EncodeToString(enc), v.PeerId,
	)
	return ws.WriteMessage(websocket.TextMessage, []byte(raw))
}

func (s *Server) writer(ws *websocket.Conn, peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.shares) == 0 {
		return ws.Close()
	}

	for hash := range s.shares {
		msg := fmt.Sprintf(
			`{"action":"announce","info_hash":"%s","peer_id":"%s","offers":[],"numwant":10}`,
			hash, peerID,
		)
		if err := ws.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) HasShares() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.shares) > 0
}

type Message struct {
	Action   string `json:"action"`
	InfoHash string `json:"info_hash"`

	// Announce msg
	Numwant int    `json:"numwant,omitempty"`
	PeerId  string `json:"peer_id,omitempty"`
	Offers  []struct {
		OfferId string `json:"offer_id"`
		Offer   struct {
			Type string `json:"type"`
			SDP  string `json:"sdp"`
		} `json:"offer"`
	} `json:"offers,omitempty"`

	// Interval msg
	Interval   int `json:"interval,omitempty"`
	Complete   int `json:"complete,omitempty"`
	Incomplete int `json:"incomplete,omitempty"`

	// Offer msg
	OfferId string `json:"offer_id,omitempty"`
	Offer   *struct {
		Type string `json:"type"`
		SDP  string `json:"sdp"`
	} `json:"offer,omitempty"`

	// Answer msg
	ToPeerId string `json:"to_peer_id,omitempty"`
	Answer   *struct {
		Type string `json:"type"`
		SDP  string `json:"sdp"`
	} `json:"answer,omitempty"`
}
