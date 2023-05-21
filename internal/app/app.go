package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

var Version = "1.5.0"
var UserAgent = "go2rtc/" + Version

var ConfigPath string
var Info = map[string]any{
	"version": Version,
}
var daemonize bool
var pidFilePath string
var forkUser user.User

func Init() {
	var confs Config
	var version bool

	currentOS := runtime.GOOS
	username := ""

	flag.Var(&confs, "config", "go2rtc config (path to file or raw text), support multiple")
	flag.BoolVar(&version, "version", false, "Print the version of the application and exit")
	if currentOS != "windows" {
		flag.BoolVar(&daemonize, "d", false, `Run in background`)
		flag.StringVar(&pidFilePath, "pid", filepath.Join(".", "go2rtc.pid"), "PID file path")
		username = *flag.String("user", "", "Username to run")
	} else {
		daemonize = false
	}
	flag.Parse()

	if version {
		fmt.Println("Current version: ", Version)
		os.Exit(0)
	}

	if username != "" {
		tmpuser, err := user.Lookup(username)
		if err != nil {
			log.Fatal().Err(err).Msgf("Cannot lookup user %s", username)
			os.Exit(1)
		}
		forkUser = *tmpuser
	} else {
		tmpuser, err := user.Current()
		if err != nil {
			log.Fatal().Err(err)
			os.Exit(1)
		}
		forkUser = *tmpuser
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

	log.Logger = NewLogger(cfg.Mod["format"], cfg.Mod["level"])

	modules = cfg.Mod

	log.Info().Msgf("go2rtc version %s %s/%s", Version, currentOS, runtime.GOARCH)
}

func IsDaemonize() bool {
	return daemonize
}
func GetPidFilePath() string {
	return pidFilePath
}
func GetForkUserId() uint32 {
	uid, err := strconv.Atoi(forkUser.Uid)
	if err != nil {
		log.Fatal().Err(err)
		os.Exit(1)
	}
	return uint32(uid)
}
func GetForkGroupId() uint32 {
	gid, err := strconv.Atoi(forkUser.Gid)
	if err != nil {
		log.Fatal().Err(err)
		os.Exit(1)
	}
	return uint32(gid)
}
func GetLogFilepath() string {
	var cfg struct {
		Mod map[string]string `yaml:"log"`
	}

	LoadConfig(&cfg)

	return cfg.Mod["file"]
}

func NewLogger(format string, level string) zerolog.Logger {
	var writer io.Writer = os.Stdout

	if format != "json" {
		writer = zerolog.ConsoleWriter{
			Out: writer, TimeFormat: "15:04:05.000",
			NoColor: writer != os.Stdout || format == "text",
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
