package app

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
)

var (
	Version    string
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

	if daemon {
		if runtime.GOOS == "windows" {
			fmt.Println("Daemon not supported on Windows")
			os.Exit(1)
		}

		args := os.Args[1:]
		for i, arg := range args {
			if arg == "-daemon" || arg == "-d" {
				args[i] = ""
			}
		}
		// Re-run the program in background and exit
		cmd := exec.Command(os.Args[0], args...)
		if err := cmd.Start(); err != nil {
			fmt.Println(err)
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
}

func readRevisionTime() (revision, vcsTime string) {
	if info, ok := debug.ReadBuildInfo(); ok {
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
					revision = "mod." + revision
				}
			}
		}
	}
	return
}
