package webtorrent

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/internal/webrtc"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webtorrent"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod struct {
			Trackers []string `yaml:"trackers"`
			Shares   map[string]struct {
				Pwd string `yaml:"pwd"`
				Src string `yaml:"src"`
			} `yaml:"shares"`
		} `yaml:"webtorrent"`
	}

	cfg.Mod.Trackers = []string{"wss://tracker.openwebtorrent.com"}

	app.LoadConfig(&cfg)

	if len(cfg.Mod.Trackers) == 0 {
		return
	}

	log = app.GetLogger("webtorrent")

	streams.HandleFunc("webtorrent", streamHandle)

	api.HandleFunc("api/webtorrent", apiHandle)

	srv = &webtorrent.Server{
		URL: cfg.Mod.Trackers[0],
		Exchange: func(src, offer string) (answer string, err error) {
			stream := streams.Get(src)
			if stream == nil {
				return "", errors.New(api.StreamNotFound)
			}
			return webrtc.ExchangeSDP(stream, offer, "webtorrent", "")
		},
	}

	if log.Debug().Enabled() {
		srv.Listen(func(msg any) {
			switch msg.(type) {
			case string, error:
				log.Debug().Msgf("[webtorrent] %s", msg)
			case *webtorrent.Message:
				log.Trace().Any("msg", msg).Msgf("[webtorrent]")
			}
		})
	}

	for name, share := range cfg.Mod.Shares {
		if len(name) < 8 {
			log.Warn().Str("name", name).Msgf("min share name len - 8 symbols")
			continue
		}
		if len(share.Pwd) < 4 {
			log.Warn().Str("name", name).Str("pwd", share.Pwd).Msgf("min share pwd len - 4 symbols")
			continue
		}
		if streams.Get(share.Src) == nil {
			log.Warn().Str("stream", share.Src).Msgf("stream not exists")
			continue
		}

		srv.AddShare(name, share.Pwd, share.Src)

		// adds to GET /api/webtorrent
		shares[name] = name
	}
}

var log zerolog.Logger

var shares = map[string]string{}
var srv *webtorrent.Server

func apiHandle(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	share, ok := shares[src]

	switch r.Method {
	case "GET":
		// support act as WebTorrent tracker (for testing purposes)
		if r.Header.Get("Connection") == "Upgrade" {
			tracker(w, r)
			return
		}

		if src != "" {
			// response one share
			if ok {
				pwd := srv.GetSharePwd(share)
				data := fmt.Sprintf(`{"share":%q,"pwd":%q}`, share, pwd)
				_, _ = w.Write([]byte(data))
			} else {
				http.Error(w, "", http.StatusNotFound)
			}
		} else {
			// response all shares
			var items []*api.Source
			for src, share := range shares {
				pwd := srv.GetSharePwd(share)
				source := fmt.Sprintf("webtorrent:?share=%s&pwd=%s", share, pwd)
				items = append(items, &api.Source{ID: src, URL: source})
			}
			api.ResponseSources(w, items)
		}

	case "POST":
		// check if share already exist
		if ok {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		// check if stream exists
		if stream := streams.Get(src); stream == nil {
			http.Error(w, "", http.StatusNotFound)
			return
		}

		// create new random share
		share = core.RandString(10, 62)
		pwd := core.RandString(10, 62)
		srv.AddShare(share, pwd, src)

		shares[src] = share

		w.WriteHeader(http.StatusCreated)
		data := fmt.Sprintf(`{"share":%q,"pwd":%q}`, share, pwd)
		api.Response(w, data, api.MimeJSON)

	case "DELETE":
		if ok {
			srv.RemoveShare(share)
			delete(shares, src)
		} else {
			http.Error(w, "", http.StatusNotFound)
		}
	}
}

func streamHandle(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	share := query.Get("share")
	pwd := query.Get("pwd")
	if len(share) < 8 || len(pwd) < 4 {
		return nil, errors.New("wrong URL: " + rawURL)
	}

	pc, err := webrtc.PeerConnection(true)
	if err != nil {
		return nil, err
	}

	return webtorrent.NewClient(srv.URL, share, pwd, pc)
}
