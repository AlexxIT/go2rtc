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

func Get(name string) *Stream {
	return streams[name]
}

var sanitize = regexp.MustCompile(`\s`)

// Validate - not allow creating dynamic streams with spaces in the source
func Validate(source string) error {
	if sanitize.MatchString(source) {
		return errors.New("streams: invalid dynamic source")
	}
	return nil
}

func New(name string, source string) *Stream {
	if Validate(source) != nil {
		return nil
	}

	stream := NewStream(source)

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

	// check an existing stream with this name
	if stream, ok := streams[name]; ok {
		stream.SetSource(source)
		return stream
	}

	// create new stream with this name
	return New(name, source)
}

func GetOrPatch(query url.Values) *Stream {
	// check if src param exists
	source := query.Get("src")
	if source == "" {
		return nil
	}

	// check if src is stream name
	if stream, ok := streams[source]; ok {
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

func GetAll() (names []string) {
	for name := range streams {
		names = append(names, name)
	}
	return
}

func Streams() map[string]*Stream {
	return streams
}

func Delete(id string) {
	delete(streams, id)
}

var log zerolog.Logger
var streams = map[string]*Stream{}
var streamsMu sync.Mutex
