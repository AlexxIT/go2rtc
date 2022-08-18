package streams

import (
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/rs/zerolog"
)

var Streams = map[string]*Stream{}

func Init() {
	var cfg struct {
		Mod map[string]interface{} `yaml:"streams"`
	}

	app.LoadConfig(&cfg)

	log = app.GetLogger("streams")

	for name, item := range cfg.Mod {
		Streams[name] = NewStream(item)
	}
}

func Get(name string) *Stream {
	return Streams[name]
}

var log zerolog.Logger
