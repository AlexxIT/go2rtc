package shell

import (
	"flag"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/rs/zerolog/log"
)

var Confs Config
var Version bool
var Daemonize bool
var PidFilePath string
var ForkUser user.User

func Init() {
	currentOS := runtime.GOOS
	var username string

	flag.Var(&Confs, "config", "go2rtc config (path to file or raw text), support multiple")
	flag.BoolVar(&Version, "version", false, "Print the version of the application and exit")
	if currentOS != "windows" {
		flag.BoolVar(&Daemonize, "d", false, `Run in background`)
		flag.StringVar(&PidFilePath, "pid", filepath.Join(".", "go2rtc.pid"), "PID file path")
		flag.StringVar(&username, "user", "", "Username to run")
	} else {
		Daemonize = false
	}
	flag.Parse()

	if username != "" {
		tmpuser, err := user.Lookup(username)
		if err != nil {
			log.Fatal().Err(err).Msgf("Cannot lookup user %s", username)
			os.Exit(1)
		}
		ForkUser = *tmpuser
	} else {
		tmpuser, err := user.Current()
		if err != nil {
			log.Fatal().Err(err)
			os.Exit(1)
		}
		ForkUser = *tmpuser
	}

}

func QuoteSplit(s string) []string {
	var a []string

	for len(s) > 0 {
		is := strings.IndexByte(s, ' ')
		if is >= 0 {
			// skip prefix and double spaces
			if is == 0 {
				// goto next symbol
				s = s[1:]
				continue
			}

			// check if quote in word
			if i := strings.IndexByte(s[:is], '"'); i >= 0 {
				// search quote end
				if is = strings.Index(s, `" `); is > 0 {
					is += 1
				} else {
					is = -1
				}
			}
		}

		if is >= 0 {
			a = append(a, strings.ReplaceAll(s[:is], `"`, ""))
			s = s[is+1:]
		} else {
			//add last word
			a = append(a, s)
			break
		}
	}
	return a
}

func GetForkUserId() uint32 {
	uid, err := strconv.Atoi(ForkUser.Uid)
	if err != nil {
		log.Fatal().Err(err)
		os.Exit(1)
	}
	return uint32(uid)
}
func GetForkGroupId() uint32 {
	gid, err := strconv.Atoi(ForkUser.Gid)
	if err != nil {
		log.Fatal().Err(err)
		os.Exit(1)
	}
	return uint32(gid)
}

// ReplaceEnvVars - support format ${CAMERA_PASSWORD} and ${RTSP_USER:admin}
func ReplaceEnvVars(text string) string {
	re := regexp.MustCompile(`\${([^}{]+)}`)
	return re.ReplaceAllStringFunc(text, func(match string) string {
		key := match[2 : len(match)-1]

		var def string
		var dok bool

		if i := strings.IndexByte(key, ':'); i > 0 {
			key, def = key[:i], key[i+1:]
			dok = true
		}

		if value, vok := os.LookupEnv(key); vok {
			return value
		}

		if dok {
			return def
		}

		return match
	})
}

func RunUntilSignal() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGSEGV)
	println("exit with signal:", (<-sigs).String())
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
