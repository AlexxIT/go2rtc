package streams

import (
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/app/store"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod map[string]interface{} `yaml:"streams"`
	}

	app.LoadConfig(&cfg)

	log = app.GetLogger("streams")

	for name, item := range cfg.Mod {
		streams[name] = NewStream(item)
	}

	for name, item := range store.GetDict("streams") {
		streams[name] = NewStream(item)
	}
}

func Get(name string) *Stream {
	return streams[name]
}

func New(name string, source interface{}) *Stream {
	stream := NewStream(source)
	streams[name] = stream
	return stream
}

func GetOrNew(src string) *Stream {
	if stream, ok := streams[src]; ok {
		return stream
	}

	if !HasProducer(src) {
		return nil
	}

	log.Info().Str("url", src).Msg("[streams] create new stream")

	return New(src, src)
}

func Delete(name string) {
	delete(streams, name)
}

func All() map[string]interface{} {
	all := map[string]interface{}{}
	for name, stream := range streams {
		all[name] = stream
		//if stream.Active() {
		//	all[name] = stream
		//}
	}
	return all
}

var log zerolog.Logger
var streams = map[string]*Stream{}
