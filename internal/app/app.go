package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
	"github.com/rs/zerolog/log"
)

var Version = "1.9.1"
var UserAgent = "go2rtc/" + Version

var ConfigPath string
var Info = map[string]any{
	"version": Version,
}

func Init() {
	var confs Config
	var daemon bool
	var version bool

	flag.Var(&confs, "config", "go2rtc config (path to file or raw text), support multiple")
	if runtime.GOOS != "windows" {
		flag.BoolVar(&daemon, "daemon", false, "Run program in background")
	}
	flag.BoolVar(&version, "version", false, "Print the version of the application and exit")
	flag.Parse()

	if version {
		vcsRevision := ""
		vcsTime := time.Now().Local()
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					if len(setting.Value) > 7 {
						vcsRevision = setting.Value[:7]
					} else {
						vcsRevision = setting.Value
					}
					vcsRevision = "(" + vcsRevision + ")"
				}
				if setting.Key == "vcs.time" {
					vcsTime, _ = time.Parse(time.RFC3339, setting.Value)
					vcsTime = vcsTime.Local()
				}
			}
		}
		fmt.Printf("go2rtc version %s%s: %s %s/%s\n", Version, vcsRevision, vcsTime.Local().String(), runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	if daemon {
		args := os.Args[1:]
		for i, arg := range args {
			if arg == "-daemon" {
				args[i] = ""
			}
		}
		// Re-run the program in background and exit
		cmd := exec.Command(os.Args[0], args...)
		if err := cmd.Start(); err != nil {
			log.Fatal().Err(err).Send()
		}
		fmt.Println("Running in daemon mode with PID:", cmd.Process.Pid)
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

	log.Logger = NewLogger(cfg.Mod["format"], cfg.Mod["level"])

	modules = cfg.Mod

	platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	log.Info().Str("version", Version).Str("platform", platform).Msg("go2rtc")
	log.Debug().Str("version", runtime.Version()).Msg("build")

	if ConfigPath != "" {
		log.Info().Str("path", ConfigPath).Msg("config")
	}

	migrateStore()
}

func LoadConfig(v any) {
	for _, data := range configs {
		if err := yaml.Unmarshal(data, v); err != nil {
			log.Warn().Err(err).Msg("[app] read config")
		}
	}
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
