package app

import (
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"runtime"
)

func Init() {
	data, _ = os.ReadFile("go2rtc.yaml")

	var cfg struct {
		Mod map[string]string `yaml:"log"`
	}

	LoadConfig(&cfg)

	var writer io.Writer = os.Stdout

	// styles
	format := cfg.Mod["format"]
	if format != "json" {
		writer = zerolog.ConsoleWriter{
			Out: writer, TimeFormat: "15:04:05.000",
			NoColor: writer != os.Stdout || format == "text",
		}
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs

	lvl, err := zerolog.ParseLevel(cfg.Mod["level"])
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}

	log = zerolog.New(writer).With().Timestamp().Logger().Level(lvl)

	modules = cfg.Mod

	log.Info().Msgf("go2rtc %s/%s", runtime.GOOS, runtime.GOARCH)
}

func LoadConfig(v interface{}) {
	if data != nil {
		_ = yaml.Unmarshal(data, v)
	}
}

func GetLogger(module string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(modules[module])
	if err != nil {
		return log
	}
	return log.Level(lvl)
}

// internal

// data - config content
var data []byte

// log - main logger
var log zerolog.Logger

// modules log levels
var modules map[string]string
