package app

import (
	"flag"
	"io"
	"os"
	"runtime"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

var Version = "0.1-rc.6"
var UserAgent = "go2rtc/" + Version
var config = flag.String(
	"config",
	"go2rtc.yaml",
	"Path to go2rtc configuration file",
)

func Init() {

	flag.Parse()

	data, _ = os.ReadFile(*config)

	var cfg struct {
		Mod map[string]string `yaml:"log"`
	}

	if data != nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			println("ERROR: " + err.Error())
		}
	}

	log.Logger = NewLogger(cfg.Mod["format"], cfg.Mod["level"])

	modules = cfg.Mod

	log.Info().Msgf("go2rtc version %s %s/%s", Version, runtime.GOOS, runtime.GOARCH)

	path, _ := os.Getwd()
	log.Debug().Str("cwd", path).Send()
}

func NewLogger(format string, level string) zerolog.Logger {
	var writer io.Writer = os.Stdout

	if format != "json" {
		writer = zerolog.ConsoleWriter{
			Out: writer, TimeFormat: "15:04:05.000",
			NoColor: writer != os.Stdout || format == "text",
		}
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs

	lvl, err := zerolog.ParseLevel(level)
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}

	return zerolog.New(writer).With().Timestamp().Logger().Level(lvl)
}

func LoadConfig(v interface{}) {
	if data != nil {
		if err := yaml.Unmarshal(data, v); err != nil {
			log.Warn().Err(err).Msg("[app] read config")
		}
	}
}

func GetConfigPath() string {
	return *config
}

func GetLogger(module string) zerolog.Logger {
	if s, ok := modules[module]; ok {
		lvl, err := zerolog.ParseLevel(s)
		if err == nil {
			return log.Level(lvl)
		}
		log.Warn().Err(err).Caller().Send()
	}

	return log.Logger
}

// internal

// data - config content
var data []byte

// modules log levels
var modules map[string]string
