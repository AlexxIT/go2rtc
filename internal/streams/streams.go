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

func GetFromSource(source string) []*Stream {
    var foundStreams []*Stream

    for _, stream := range streams {
        for _, src := range stream.Sources() {
            if src == source {
                foundStreams = append(foundStreams, stream)
                break
            }
        }
    }

	if len(foundStreams) == 0 {
		return nil
	}

    return foundStreams
}

var sanitize = regexp.MustCompile(`\s`)

// Validate - not allow creating dynamic streams with spaces in the source
func Validate(source string) error {
	if sanitize.MatchString(source) {
		return errors.New("streams: invalid dynamic source")
	}
	return nil
}

func New(name string, source any) *Stream {
	switch source := source.(type) {
	case string:
		if Validate(source) != nil {
			return nil
		}

		stream := NewStream(source)
		streams[name] = stream

		return stream

	case []string:
		for _, s := range source {
			if Validate(s) != nil {
				return nil
			}
		}

		stream := NewStream(source)
		streams[name] = stream

		return stream

	case nil:
		stream := NewStream(nil)
		streams[name] = stream

		return stream

	default:
		return nil
	}
}

func Patch(name string, sources ...string) *Stream {
    streamsMu.Lock()
    defer streamsMu.Unlock()

	if len(sources) == 0 {
		return New(name, nil)
	}

    var stream *Stream

    for _, source := range sources {
        // check if source links to some stream name from go2rtc
        if u, err := url.Parse(source); err == nil && u.Scheme == "rtsp" && len(u.Path) > 1 {
            rtspName := u.Path[1:]
            if s, ok := streams[rtspName]; ok {
                if stream == nil {
                    stream = s
                } else if stream != s {
                    // link (alias) streams[name] to streams[rtspName]
                    streams[name] = s
                    return s
                }
            }
        }

        if s, ok := streams[source]; ok {
            if stream == nil {
                stream = s
            } else if stream != s {
                // link (alias) streams[name] to streams[source]
                streams[name] = s
                return s
            }
        }

        // check if src has supported scheme
        if !HasProducer(source) {
            return nil
        }
    }

    // check an existing stream with this name
    if s, ok := streams[name]; ok {
        if stream == nil {
            stream = s
        }
        if len(sources) == 1 {
            stream.SetSource(sources[0])
        } else {
            stream.SetSources(sources)
        }
        return stream
    }

    // create new stream with this name
    if len(sources) == 1 {
        return New(name, sources[0])
    }

    return New(name, sources)
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
