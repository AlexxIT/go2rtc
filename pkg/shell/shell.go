package shell

import (
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
)

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
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	println("exit with signal:", (<-sigs).String())
}
