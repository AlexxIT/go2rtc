package homekit

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/srtp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/homekit"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod map[string]struct {
			Pin           string   `yaml:"pin"`
			Name          string   `yaml:"name"`
			DeviceID      string   `yaml:"device_id"`
			DevicePrivate string   `yaml:"device_private"`
			Pairings      []string `yaml:"pairings"`
		} `yaml:"homekit"`
	}
	app.LoadConfig(&cfg)

	log = app.GetLogger("homekit")

	streams.HandleFunc("homekit", streamHandler)

	api.HandleFunc("api/homekit", apiHandler)

	if cfg.Mod == nil {
		return
	}

	servers = map[string]*server{}
	var entries []*mdns.ServiceEntry

	for id, conf := range cfg.Mod {
		stream := streams.Get(id)
		if stream == nil {
			log.Warn().Msgf("[homekit] missing stream: %s", id)
			continue
		}

		if conf.Pin == "" {
			conf.Pin = "19550224" // default PIN
		}

		pin, err := hap.SanitizePin(conf.Pin)
		if err != nil {
			log.Error().Err(err).Caller().Send()
			continue
		}

		deviceID := calcDeviceID(conf.DeviceID, id) // random MAC-address
		name := calcName(conf.Name, deviceID)

		srv := &server{
			stream:   id,
			srtp:     srtp.Server,
			pairings: conf.Pairings,
		}

		srv.hap = &hap.Server{
			Pin:           pin,
			DeviceID:      deviceID,
			DevicePrivate: calcDevicePrivate(conf.DevicePrivate, id),
			GetPair:       srv.GetPair,
			AddPair:       srv.AddPair,
			Handler:       homekit.ServerHandler(srv),
		}

		if url := findHomeKitURL(stream); url != "" {
			// 1. Act as transparent proxy for HomeKit camera
			dial := func() (net.Conn, error) {
				client, err := homekit.Dial(url, srtp.Server)
				if err != nil {
					return nil, err
				}
				return client.Conn(), nil
			}
			srv.hap.Handler = homekit.ProxyHandler(srv, dial)
		} else {
			// 2. Act as basic HomeKit camera
			srv.accessory = camera.NewAccessory("AlexxIT", "go2rtc", name, "-", app.Version)
			srv.hap.Handler = homekit.ServerHandler(srv)
		}

		srv.mdns = &mdns.ServiceEntry{
			Name: name,
			Port: uint16(api.Port),
			Info: map[string]string{
				hap.TXTConfigNumber: "1",
				hap.TXTFeatureFlags: "0",
				hap.TXTDeviceID:     deviceID,
				hap.TXTModel:        app.UserAgent,
				hap.TXTProtoVersion: "1.1",
				hap.TXTStateNumber:  "1",
				hap.TXTStatusFlags:  hap.StatusNotPaired,
				hap.TXTCategory:     hap.CategoryCamera,
				hap.TXTSetupHash:    srv.hap.SetupHash(),
			},
		}
		entries = append(entries, srv.mdns)

		srv.UpdateStatus()

		host := srv.mdns.Host(mdns.ServiceHAP)
		servers[host] = srv
	}

	api.HandleFunc(hap.PathPairSetup, hapPairSetup)
	api.HandleFunc(hap.PathPairVerify, hapPairVerify)

	log.Trace().Msgf("[homekit] mdns: %s", entries)

	go func() {
		if err := mdns.Serve(mdns.ServiceHAP, entries); err != nil {
			log.Error().Err(err).Caller().Send()
		}
	}()
}

var log zerolog.Logger
var servers map[string]*server

func streamHandler(rawURL string) (core.Producer, error) {
	if srtp.Server == nil {
		return nil, errors.New("homekit: can't work without SRTP server")
	}

	rawURL, rawQuery, _ := strings.Cut(rawURL, "#")
	client, err := homekit.Dial(rawURL, srtp.Server)
	if client != nil && rawQuery != "" {
		query := streams.ParseQuery(rawQuery)
		client.Bitrate = parseBitrate(query.Get("bitrate"))
	}

	return client, err
}

func hapPairSetup(w http.ResponseWriter, r *http.Request) {
	srv, ok := servers[r.Host]
	if !ok {
		log.Error().Msg("[homekit] unknown host: " + r.Host)
		return
	}

	conn, rw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}

	defer conn.Close()

	if err = srv.hap.PairSetup(r, rw, conn); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func hapPairVerify(w http.ResponseWriter, r *http.Request) {
	srv, ok := servers[r.Host]
	if !ok {
		log.Error().Msg("[homekit] unknown host: " + r.Host)
		return
	}

	conn, rw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}

	defer conn.Close()

	if err = srv.hap.PairVerify(r, rw, conn); err != nil && err != io.EOF {
		log.Error().Err(err).Caller().Send()
	}
}

func findHomeKitURL(stream *streams.Stream) string {
	sources := stream.Sources()
	if len(sources) == 0 {
		return ""
	}

	url := sources[0]
	if strings.HasPrefix(url, "homekit") {
		return url
	}

	if strings.HasPrefix(url, "hass") {
		location, _ := streams.Location(url)
		if strings.HasPrefix(location, "homekit") {
			return url
		}
	}

	return ""
}

func parseBitrate(s string) int {
	n := len(s)
	if n == 0 {
		return 0
	}

	var k int
	switch n--; s[n] {
	case 'K':
		k = 1024
		s = s[:n]
	case 'M':
		k = 1024 * 1024
		s = s[:n]
	default:
		k = 1
	}

	return k * core.Atoi(s)
}
