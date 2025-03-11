package streams

import (
	"errors"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Streams map[string]any `yaml:"streams"`
		Publish map[string]any `yaml:"publish"`
	}

	app.LoadConfig(&cfg)

	log = app.GetLogger("streams")

	for name, item := range cfg.Streams {
		streams[name] = NewStream(item)
	}

	api.HandleFunc("api/streams", apiStreams)
	api.HandleFunc("api/streams.dot", apiStreamsDOT)

	if cfg.Publish == nil {
		return
	}

	time.AfterFunc(time.Second, func() {
		for name, dst := range cfg.Publish {
			if stream := Get(name); stream != nil {
				Publish(stream, dst)
			}
		}
	})
}

var sanitize = regexp.MustCompile(`\s`)

// Validate - not allow creating dynamic streams with spaces in the source
func Validate(source string) error {
	if sanitize.MatchString(source) {
		return errors.New("streams: invalid dynamic source")
	}
	return nil
}

func New(name string, sources ...string) *Stream {
	for _, source := range sources {
		if Validate(source) != nil {
			return nil
		}
	}

	stream := NewStream(sources)

	streamsMu.Lock()
	streams[name] = stream
	streamsMu.Unlock()

	return stream
}

func Patch(name string, source string) *Stream {
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
			return stream
		}
	}

	if stream, ok := streams[source]; ok {
		if name != source {
			// link (alias) streams[name] to streams[source]
			streams[name] = stream
		}
		return stream
	}

	// check if src has supported scheme
	if !HasProducer(source) {
		return nil
	}

	if Validate(source) != nil {
		return nil
	}

	// check an existing stream with this name
	if stream, ok := streams[name]; ok {
		stream.SetSource(source)
		return stream
	}

	// create new stream with this name
	stream := NewStream(source)
	streams[name] = stream
	return stream
}

func GetOrPatch(query url.Values) *Stream {
	// check if src param exists
	source := query.Get("src")
	if source == "" {
		return nil
	}

	// check if src is stream name
	if stream := Get(source); stream != nil {
		return stream
	}

	// check if name param provided
	if name := query.Get("name"); name != "" {
		log.Info().Msgf("[streams] create new stream url=%s", source)

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
