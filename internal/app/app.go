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
	flag.Var(&confs, "c", "")
	if runtime.GOOS != "windows" {
		flag.BoolVar(&daemon, "daemon", false, "Run program in background")
	}
	flag.BoolVar(&version, "version", false, "Print the version of the application and exit")
	flag.BoolVar(&version, "v", false, "")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of go2rtc\nversion %s\n\n", GetVersionString())
		flag.VisitAll(func(f *flag.Flag) {
			pname := ""
			if f.Usage != "" {
				switch f.Name {
				case "config":
					pname = "-c --config"
					break
				case "daemon":
					pname = "-d --daemon"
					break
				case "version":
					pname = "-v --version"
					break
				default:
					pname = "-" + f.Name
				}
				fmt.Fprintf(os.Stderr, "\t%s\n\t\t%s (default %q)\n", pname, f.Usage, f.DefValue)
			}
		})
		fmt.Fprintf(os.Stderr, "\t%s\n\t\t%s\n", "-h --help", "Print this help")
	}

	flag.Parse()

	if version {
		fmt.Println("go2rtc version " + GetVersionString())
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

func GetVersionString() string {
	var vcsRevision string
	vcsTime := time.Now()

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				vcsRevision = setting.Value
				if len(vcsRevision) > 7 {
					vcsRevision = vcsRevision[:7]
				}
				vcsRevision = "(" + vcsRevision + ")"
			case "vcs.time":
				if parsedTime, err := time.Parse(time.RFC3339, setting.Value); err == nil {
					vcsTime = parsedTime.Local()
				}
			}
		}
	}

	return fmt.Sprintf("%s%s: %s %s/%s", Version, vcsRevision, vcsTime.String(), runtime.GOOS, runtime.GOARCH)
}
