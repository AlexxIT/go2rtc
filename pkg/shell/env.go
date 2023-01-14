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
		i := strings.IndexByte(key, ':')
		if i > 0 {
			key, def = key[:i], key[i+1:]
		}

		value, exists := os.LookupEnv(key)
		if exists {
			return value
		}

		if def != "" {
			return def
		}

		return match
	})
}
