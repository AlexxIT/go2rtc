package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var Version = "1.8.4"
var UserAgent = "go2rtc/" + Version

var ConfigPath string
var LogFilePath string
var Info = map[string]any{
	"version": Version,
}

func Init() {
	var confs Config
	var version bool

	flag.Var(&confs, "config", "go2rtc config (path to file or raw text), support multiple")
	flag.BoolVar(&version, "version", false, "Print the version of the application and exit")
	flag.Parse()

	if version {
		fmt.Println("Current version: ", Version)
		os.Exit(0)
	}

	if confs == nil {
		confs = []string{"go2rtc.yaml"}
	}

	for _, conf := range confs {
		if conf[0] != '{' {
			// config as file
			if ConfigPath == "" {
				ConfigPath = conf
			}

			data, _ := os.ReadFile(conf)
			if data == nil {
				continue
			}

			data = []byte(shell.ReplaceEnvVars(string(data)))
			configs = append(configs, data)
		} else {
			// config as raw YAML
			configs = append(configs, []byte(conf))
		}
	}

	if ConfigPath != "" {
		if !filepath.IsAbs(ConfigPath) {
			if cwd, err := os.Getwd(); err == nil {
				ConfigPath = filepath.Join(cwd, ConfigPath)
			}
		}
		Info["config_path"] = ConfigPath
	}

	var cfg struct {
		Mod map[string]string `yaml:"log"`
	}

	LoadConfig(&cfg)

	log.Logger = NewLogger(cfg.Mod["format"], cfg.Mod["level"], GetLogFilepath())

	modules = cfg.Mod

	log.Info().Msgf("go2rtc version %s %s/%s", Version, runtime.GOOS, runtime.GOARCH)
	log.Debug().Msgf("[log] file: %s", GetLogFilepath())

	migrateStore()
}

// GetLogFilepath retrieves the file path for the log file from the application's configuration.
// The configuration is expected to be in YAML format and contain a "log" section with a "file" key.
// It uses the LoadConfig function to populate the cfg structure with the configuration data.
//
// Returns:
//
//	string: The file path of the log file as specified in the configuration.
//
// Note:
//
//	The function assumes that the LoadConfig function is defined elsewhere and is responsible
//	for loading and parsing the configuration into the provided struct.
//	The cfg struct is an anonymous struct with a Mod field, which is a map with string keys and values.
//	The "log" key within the Mod map is expected to contain a sub-map with the "file" key that holds the log file path.
//
// Example of expected YAML configuration:
//
//	log:
//	  file: "/path/to/logfile.log"
//
// If the "file" key is not found within the "log" section of the configuration, the function will return an empty string.
func GetLogFilepath() string {
	var cfg struct {
		Mod map[string]string `yaml:"log"`
	}

	if LogFilePath != "" {
		return LogFilePath
	}

	LoadConfig(&cfg)

	if cfg.Mod["file"] == "" {
		// Generate temporary log file
		tmpFile, err := ioutil.TempFile("", "go2rtc*.log")
		if err != nil {
			return ""
		}
		defer tmpFile.Close()

		LogFilePath = tmpFile.Name()

	} else {
		LogFilePath = cfg.Mod["file"]
	}

	return LogFilePath
}

func NewLogger(format string, level string, file string) zerolog.Logger {
	var writer io.Writer = os.Stdout

	if format != "json" {
		writer = zerolog.ConsoleWriter{
			Out: writer, TimeFormat: "15:04:05.000",
			NoColor: writer != os.Stdout || format == "text",
		}
	}

	if file != "" {
		fileHandler, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		fileLogger := zerolog.ConsoleWriter{
			Out: fileHandler, TimeFormat: "15:04:05.000",
			NoColor: true,
		}

		if err == nil {
			writer = zerolog.MultiLevelWriter(writer, fileLogger)
		}

	}

	zerolog.TimeFieldFormat = time.RFC3339Nano

	lvl, err := zerolog.ParseLevel(level)
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}

	return zerolog.New(writer).With().Timestamp().Logger().Level(lvl)
}

func LoadConfig(v any) {
	for _, data := range configs {
		if err := yaml.Unmarshal(data, v); err != nil {
			log.Warn().Err(err).Msg("[app] read config")
		}
	}
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

func PatchConfig(key string, value any, path ...string) error {
	if ConfigPath == "" {
		return errors.New("config file disabled")
	}

	// empty config is OK
	b, _ := os.ReadFile(ConfigPath)

	b, err := yaml.Patch(b, key, value, path...)
	if err != nil {
		return err
	}

	return os.WriteFile(ConfigPath, b, 0644)
}

// internal

type Config []string

func (c *Config) String() string {
	return strings.Join(*c, " ")
}

func (c *Config) Set(value string) error {
	*c = append(*c, value)
	return nil
}

var configs [][]byte

// modules log levels
var modules map[string]string
