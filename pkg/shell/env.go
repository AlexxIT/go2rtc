package shell

import (
	"os"
	"regexp"
	"strings"
)

func ReplaceEnvVars(text string) string {
	re := regexp.MustCompile(`\${([^}{]+)}`)
	return re.ReplaceAllStringFunc(text, func(match string) string {
		key := match[2 : len(match)-1]

		var def string
		var dok bool

		i := strings.IndexByte(key, ':')
		if i > 0 {
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
