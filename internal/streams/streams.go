package streams

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/rtp"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Streams map[string]any    `yaml:"streams"`
		Publish map[string]any    `yaml:"publish"`
		Preload map[string]string `yaml:"preload"`
	}

	app.LoadConfig(&cfg)

	log = app.GetLogger("streams")

	for name, item := range cfg.Streams {
		streams[name] = NewStream(item)
	}

	api.HandleFunc("api/streams", apiStreams)
	api.HandleFunc("api/streams.dot", apiStreamsDOT)
	api.HandleFunc("api/preload", apiPreload)
	api.HandleFunc("api/schemes", apiSchemes)

	// Initialize RTP bidirectional endpoints
	initRTPEndpoints(cfg.Streams)

	if cfg.Publish == nil && cfg.Preload == nil {
		return
	}

	time.AfterFunc(time.Second, func() {
		// range for nil map is OK
		for name, dst := range cfg.Publish {
			if stream := Get(name); stream != nil {
				Publish(stream, dst)
			}
		}
		for name, rawQuery := range cfg.Preload {
			if err := AddPreload(name, rawQuery); err != nil {
				log.Error().Err(err).Caller().Send()
			}
		}
	})
}

func New(name string, sources ...string) (*Stream, error) {
	for _, source := range sources {
		if !HasProducer(source) {
			return nil, errors.New("streams: source not supported")
		}

		if err := Validate(source); err != nil {
			return nil, err
		}
	}

	stream := NewStream(sources)

	streamsMu.Lock()
	streams[name] = stream
	streamsMu.Unlock()

	return stream, nil
}

func Patch(name string, source string) (*Stream, error) {
	streamsMu.Lock()
	defer streamsMu.Unlock()

	// check if source links to some stream name from go2rtc
	if u, err := url.Parse(source); err == nil && u.Scheme == "rtsp" && len(u.Path) > 1 {
		rtspName := u.Path[1:]
		if stream, ok := streams[rtspName]; ok {
			if streams[name] != stream {
				// link (alias) streams[name] to streams[rtspName]
				streams[name] = stream
			}
			return stream, nil
		}
	}

	if stream, ok := streams[source]; ok {
		if name != source {
			// link (alias) streams[name] to streams[source]
			streams[name] = stream
		}
		return stream, nil
	}

	// check if src has supported scheme
	if !HasProducer(source) {
		return nil, errors.New("streams: source not supported")
	}

	if err := Validate(source); err != nil {
		return nil, err
	}

	// check an existing stream with this name
	if stream, ok := streams[name]; ok {
		stream.SetSource(source)
		return stream, nil
	}

	// create new stream with this name
	stream := NewStream(source)
	streams[name] = stream
	return stream, nil
}

func GetOrPatch(query url.Values) (*Stream, error) {
	// check if src param exists
	source := query.Get("src")
	if source == "" {
		return nil, errors.New("streams: source empty")
	}

	// check if src is stream name
	if stream := Get(source); stream != nil {
		return stream, nil
	}

	// check if name param provided
	if name := query.Get("name"); name != "" {
		return Patch(name, source)
	}

	// return new stream with src as name
	return Patch(source, source)
}

var log zerolog.Logger

// streams map

var streams = map[string]*Stream{}
var streamsMu sync.Mutex

func Get(name string) *Stream {
	streamsMu.Lock()
	defer streamsMu.Unlock()
	return streams[name]
}

func Delete(name string) {
	streamsMu.Lock()
	defer streamsMu.Unlock()
	delete(streams, name)
}

func GetAllNames() []string {
	streamsMu.Lock()
	names := make([]string, 0, len(streams))
	for name := range streams {
		names = append(names, name)
	}
	streamsMu.Unlock()
	return names
}

func GetAllSources() map[string][]string {
	streamsMu.Lock()
	sources := make(map[string][]string, len(streams))
	for name, stream := range streams {
		sources[name] = stream.Sources()
	}
	streamsMu.Unlock()
	return sources
}

// initRTPEndpoints processes stream configurations and sets up bidirectional RTP endpoints
func initRTPEndpoints(streamsCfg map[string]any) {
	for streamName, item := range streamsCfg {
		sources := itemToSources(item)
		for _, source := range sources {
			if !strings.HasPrefix(source, "rtp://") {
				continue
			}

			stream := Get(streamName)
			if stream == nil {
				log.Warn().Msgf("[streams] stream %s not found for rtp endpoint", streamName)
				continue
			}

			rtpEndpoint, err := parseRTPURL(source)
			if err != nil {
				log.Error().Err(err).Msgf("[streams] failed to parse rtp url: %s", source)
				continue
			}

			if err := stream.AddConsumer(rtpEndpoint); err != nil {
				log.Error().Err(err).Msgf("[streams] failed to add rtp consumer to stream %s", streamName)
				continue
			}

			log.Info().Str("stream", streamName).Str("remote", rtpEndpoint.remoteAddr.String()).Msg("[streams] added rtp bidirectional endpoint")
		}
	}
}

func itemToSources(item any) []string {
	switch v := item.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []any:
		var sources []string
		for _, s := range v {
			if str, ok := s.(string); ok {
				sources = append(sources, str)
			}
		}
		return sources
	default:
		return nil
	}
}

type rtpEndpoint struct {
	*rtp.RTP
	remoteAddr *net.UDPAddr
}

func parseRTPURL(rawURL string) (*rtpEndpoint, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	remoteHost := u.Hostname()
	remotePort, _ := strconv.Atoi(u.Port())
	if remotePort == 0 {
		remotePort = 5000 // Default RTP port
	}

	localPort, _ := strconv.Atoi(u.Query().Get("local_port"))
	if localPort == 0 {
		localPort = remotePort // Default to same port
	}

	// The remote codec is what we negotiate with the SIP/RTP caller
	// Default to PCMA for maximum compatibility (like WebRTC uses)
	codecName := u.Query().Get("codec")
	if codecName == "" {
		codecName = core.CodecPCMA
	}

	remoteCodec := &core.Codec{Name: codecName}
	if codecName == core.CodecPCMA || codecName == core.CodecPCMU {
		remoteCodec.ClockRate = 8000
	}

	// Transcoding is set up automatically in AddTrack/GetTrack when the
	// actual camera codec is known via the codec matching in AddConsumer.
	r, err := rtp.NewRTP(fmt.Sprintf("%s:%d", remoteHost, remotePort), localPort, remoteCodec)
	if err != nil {
		return nil, err
	}

	remoteAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", remoteHost, remotePort))

	return &rtpEndpoint{
		RTP:        r,
		remoteAddr: remoteAddr,
	}, nil
}
