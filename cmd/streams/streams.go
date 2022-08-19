package streams

import (
	"github.com/AlexxIT/go2rtc/cmd/app"
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
}

func Get(name string) *Stream {
	if stream, ok := streams[name]; ok {
		return stream
	}

	if HasProducer(name) {
		log.Info().Str("url", name).Msg("[streams] create new stream")
		stream := NewStream(name)
		streams[name] = stream
		return stream
	}

	return nil
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
