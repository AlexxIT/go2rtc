package streams

import (
	"net/url"
	"strings"
)

func ParseQuery(s string) url.Values {
	if len(s) == 0 {
		return nil
	}
	params := url.Values{}
	for _, key := range strings.Split(s, "#") {
		var value string
		i := strings.IndexByte(key, '=')
		if i > 0 {
			key, value = key[:i], key[i+1:]
		}
		params[key] = append(params[key], value)
	}
	return params
}
