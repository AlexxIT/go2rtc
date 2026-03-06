package homekit

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg"
	"github.com/AlexxIT/go2rtc/internal/srtp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/AlexxIT/go2rtc/pkg/hksv"
	"github.com/AlexxIT/go2rtc/pkg/homekit"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod map[string]struct {
			Pin             string   `yaml:"pin"`
			Name            string   `yaml:"name"`
			DeviceID        string   `yaml:"device_id"`
			DevicePrivate   string   `yaml:"device_private"`
			CategoryID      string   `yaml:"category_id"`
			Pairings        []string `yaml:"pairings"`
			HKSV            bool     `yaml:"hksv"`
			Motion          string   `yaml:"motion"`
			MotionThreshold float64  `yaml:"motion_threshold"`
			Speaker         *bool    `yaml:"speaker"`
		} `yaml:"homekit"`
	}
	app.LoadConfig(&cfg)

	log = app.GetLogger("homekit")

	streams.HandleFunc("homekit", streamHandler)

	api.HandleFunc("api/homekit", apiHomekit)
	api.HandleFunc("api/homekit/accessories", apiHomekitAccessories)
	api.HandleFunc("api/homekit/motion", apiMotion)
	api.HandleFunc("api/homekit/doorbell", apiDoorbell)
	api.HandleFunc("api/discovery/homekit", apiDiscovery)

	if cfg.Mod == nil {
		return
	}

	hosts = map[string]*hksv.Server{}
	servers = map[string]*hksv.Server{}
	var entries []*mdns.ServiceEntry

	for id, conf := range cfg.Mod {
		stream := streams.Get(id)
		if stream == nil {
			log.Warn().Msgf("[homekit] missing stream: %s", id)
			continue
		}

		var proxyURL string
		if url := findHomeKitURL(stream.Sources()); url != "" {
			proxyURL = url
		}

		srv, err := hksv.NewServer(hksv.Config{
			StreamName:      id,
			Pin:             conf.Pin,
			Name:            conf.Name,
			DeviceID:        conf.DeviceID,
			DevicePrivate:   conf.DevicePrivate,
			CategoryID:      conf.CategoryID,
			Pairings:        conf.Pairings,
			ProxyURL:        proxyURL,
			HKSV:            conf.HKSV,
			MotionMode:      conf.Motion,
			MotionThreshold: conf.MotionThreshold,
			Speaker:         conf.Speaker,
			UserAgent:       app.UserAgent,
			Version:         app.Version,
			Streams:         &go2rtcStreamProvider{},
			Store:           &go2rtcPairingStore{},
			Snapshots:       &go2rtcSnapshotProvider{},
			LiveStream:      &go2rtcLiveStreamHandler{},
			Logger:          log,
			Port:            uint16(api.Port),
		})
		if err != nil {
			log.Error().Err(err).Str("stream", id).Msg("[homekit] create server failed")
			continue
		}

		entry := srv.MDNSEntry()
		entries = append(entries, entry)

		host := entry.Host(mdns.ServiceHAP)
		hosts[host] = srv
		servers[id] = srv

		log.Trace().Msgf("[homekit] new server: %s", entry)
	}

	api.HandleFunc(hap.PathPairSetup, hapHandler)
	api.HandleFunc(hap.PathPairVerify, hapHandler)

	go func() {
		if err := mdns.Serve(mdns.ServiceHAP, entries); err != nil {
			log.Error().Err(err).Caller().Send()
		}
	}()
}

var log zerolog.Logger
var hosts map[string]*hksv.Server
var servers map[string]*hksv.Server

// go2rtcStreamProvider implements hksv.StreamProvider
type go2rtcStreamProvider struct{}

func (p *go2rtcStreamProvider) AddConsumer(name string, cons core.Consumer) error {
	stream := streams.Get(name)
	if stream == nil {
		return errors.New("stream not found: " + name)
	}
	return stream.AddConsumer(cons)
}

func (p *go2rtcStreamProvider) RemoveConsumer(name string, cons core.Consumer) {
	if s := streams.Get(name); s != nil {
		s.RemoveConsumer(cons)
	}
}

// go2rtcPairingStore implements hksv.PairingStore
type go2rtcPairingStore struct{}

func (s *go2rtcPairingStore) SavePairings(name string, pairings []string) error {
	return app.PatchConfig([]string{"homekit", name, "pairings"}, pairings)
}

// go2rtcSnapshotProvider implements hksv.SnapshotProvider
type go2rtcSnapshotProvider struct{}

func (s *go2rtcSnapshotProvider) GetSnapshot(streamName string, width, height int) ([]byte, error) {
	stream := streams.Get(streamName)
	if stream == nil {
		return nil, errors.New("stream not found: " + streamName)
	}

	cons := magic.NewKeyframe()
	if err := stream.AddConsumer(cons); err != nil {
		return nil, err
	}

	once := &core.OnceBuffer{}
	_, _ = cons.WriteTo(once)
	b := once.Buffer()

	stream.RemoveConsumer(cons)

	switch cons.CodecName() {
	case core.CodecH264, core.CodecH265:
		var err error
		if b, err = ffmpeg.JPEGWithScale(b, width, height); err != nil {
			return nil, err
		}
	}

	return b, nil
}

// go2rtcLiveStreamHandler implements hksv.LiveStreamHandler
type go2rtcLiveStreamHandler struct {
	mu       sync.Mutex
	consumer *homekit.Consumer
}

func (h *go2rtcLiveStreamHandler) SetupEndpoints(conn net.Conn, offer *camera.SetupEndpointsRequest) (any, error) {
	consumer := homekit.NewConsumer(conn, srtp.Server)
	consumer.SetOffer(offer)

	h.mu.Lock()
	h.consumer = consumer
	h.mu.Unlock()

	answer := consumer.GetAnswer()
	v, err := tlv8.MarshalBase64(answer)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (h *go2rtcLiveStreamHandler) GetEndpointsResponse() any {
	h.mu.Lock()
	consumer := h.consumer
	h.mu.Unlock()
	if consumer == nil {
		return nil
	}
	answer := consumer.GetAnswer()
	v, _ := tlv8.MarshalBase64(answer)
	return v
}

func (h *go2rtcLiveStreamHandler) StartStream(streamName string, conf *camera.SelectedStreamConfiguration, connTracker hksv.ConnTracker) error {
	h.mu.Lock()
	consumer := h.consumer
	h.mu.Unlock()

	if consumer == nil {
		return errors.New("no consumer")
	}

	if !consumer.SetConfig(conf) {
		return errors.New("wrong config")
	}

	connTracker.AddConn(consumer)

	stream := streams.Get(streamName)
	if err := stream.AddConsumer(consumer); err != nil {
		return err
	}

	go func() {
		_, _ = consumer.WriteTo(nil)
		stream.RemoveConsumer(consumer)
		connTracker.DelConn(consumer)
	}()

	return nil
}

func (h *go2rtcLiveStreamHandler) StopStream(sessionID string, connTracker hksv.ConnTracker) error {
	h.mu.Lock()
	consumer := h.consumer
	h.mu.Unlock()

	if consumer != nil && consumer.SessionID() == sessionID {
		_ = consumer.Stop()
	}
	return nil
}

func streamHandler(rawURL string) (core.Producer, error) {
	if srtp.Server == nil {
		return nil, errors.New("homekit: can't work without SRTP server")
	}

	rawURL, rawQuery, _ := strings.Cut(rawURL, "#")
	client, err := homekit.Dial(rawURL, srtp.Server)
	if client != nil && rawQuery != "" {
		query := streams.ParseQuery(rawQuery)
		client.MaxWidth = core.Atoi(query.Get("maxwidth"))
		client.MaxHeight = core.Atoi(query.Get("maxheight"))
		client.Bitrate = parseBitrate(query.Get("bitrate"))
	}

	return client, err
}

func resolve(host string) *hksv.Server {
	if len(hosts) == 1 {
		for _, srv := range hosts {
			return srv
		}
	}
	if srv, ok := hosts[host]; ok {
		return srv
	}
	return nil
}

func hapHandler(w http.ResponseWriter, r *http.Request) {
	srv := resolve(r.Host)
	if srv == nil {
		log.Error().Msg("[homekit] unknown host: " + r.Host)
		return
	}
	srv.Handle(w, r)
}

func findHomeKitURL(sources []string) string {
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
			return location
		}
	}

	return ""
}

func apiMotion(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	srv := servers[id]
	if srv == nil {
		http.Error(w, "server not found: "+id, http.StatusNotFound)
		return
	}
	switch r.Method {
	case "POST":
		srv.SetMotionDetected(true)
	case "DELETE":
		srv.SetMotionDetected(false)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func apiDoorbell(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	srv := servers[id]
	if srv == nil {
		http.Error(w, "server not found: "+id, http.StatusNotFound)
		return
	}
	srv.TriggerDoorbell()
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
