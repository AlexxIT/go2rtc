package app

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
)

var (
	Version    string
	Modules    []string
	UserAgent  string
	ConfigPath string
	Info       = make(map[string]any)
)

const usage = `Usage of go2rtc:

  -c, --config   Path to config file or config string as YAML or JSON, support multiple
  -d, --daemon   Run in background
  -v, --version  Print version and exit
`

func Init() {
	var config flagConfig
	var daemon bool
	var version bool

	flag.Var(&config, "config", "")
	flag.Var(&config, "c", "")
	flag.BoolVar(&daemon, "daemon", false, "")
	flag.BoolVar(&daemon, "d", false, "")
	flag.BoolVar(&version, "version", false, "")
	flag.BoolVar(&version, "v", false, "")

	flag.Usage = func() { fmt.Print(usage) }
	flag.Parse()

	revision, vcsTime := readRevisionTime()

	if version {
		fmt.Printf("go2rtc version %s (%s) %s/%s\n", Version, revision, runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	if daemon && os.Getppid() != 1 {
		if runtime.GOOS == "windows" {
			fmt.Println("Daemon mode is not supported on Windows")
			os.Exit(1)
		}

		// Re-run the program in background and exit
		cmd := exec.Command(os.Args[0], os.Args[1:]...)
		if err := cmd.Start(); err != nil {
			fmt.Println("Failed to start daemon:", err)
			os.Exit(1)
		}
		fmt.Println("Running in daemon mode with PID:", cmd.Process.Pid)
		os.Exit(0)
	}

	UserAgent = "go2rtc/" + Version

	Info["version"] = Version
	Info["revision"] = revision

	initConfig(config)
	initLogger()

	platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	Logger.Info().Str("version", Version).Str("platform", platform).Str("revision", revision).Msg("go2rtc")
	Logger.Debug().Str("version", runtime.Version()).Str("vcs.time", vcsTime).Msg("build")

	if ConfigPath != "" {
		Logger.Info().Str("path", ConfigPath).Msg("config")
	}

	var cfg struct {
		Mod struct {
			Modules []string `yaml:"modules"`
		} `yaml:"app"`
	}

	LoadConfig(&cfg)

	Modules = cfg.Mod.Modules
}

func readRevisionTime() (revision, vcsTime string) {
	if info, ok := debug.ReadBuildInfo(); ok {
		// Rewrite version from -buildvcs info if it is valid.
		// Format for tagged version: v1.9.13
		// Format for custom commit:  v1.9.14-0.20251215184105-753d6617ab58
		// Format for modified code:  v1.9.14-0.20251215184105-753d6617ab58+dirty
		if s, ok := strings.CutPrefix(info.Main.Version, "v"); ok {
			Version = s
		}

		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if len(setting.Value) > 7 {
					revision = setting.Value[:7]
				} else {
					revision = setting.Value
				}
			case "vcs.time":
				vcsTime = setting.Value
			case "vcs.modified":
				if setting.Value == "true" {
					revision += "+dirty"
				}
			}
		}
	}
	return
}
