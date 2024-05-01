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
		switch c := s[0]; c {
		case '\t', '\n', '\r', ' ': // unicode.IsSpace
			s = s[1:]
		case '"', '\'': // quote chars
			if i := strings.IndexByte(s[1:], c); i > 0 {
				a = append(a, s[1:i+1])
				s = s[i+2:]
			} else {
				return nil // error
			}
		default:
			i := strings.IndexAny(s, "\t\n\r ")
			if i > 0 {
				a = append(a, s[:i])
				s = s[i:]
			} else {
				a = append(a, s)
				s = ""
			}
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
