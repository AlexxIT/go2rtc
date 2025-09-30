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

func FindPrefixURL(prefix string, sources []string) string {
	if len(sources) == 0 {
		return ""
	}

	url := sources[0]
	if strings.HasPrefix(url, prefix) {
		return url
	}

	if prefix == "homekit" {
		if strings.HasPrefix(url, "hass") {
			location, _ := Location(url)
			if strings.HasPrefix(location, prefix) {
				return url
			}
		}
	}

	return ""
}

func FindPrefixURLs(prefix string) map[string]*url.URL {
	urls := map[string]*url.URL{}
	for name, sources := range GetAllSources() {
		if rawURL := FindPrefixURL(prefix, sources); rawURL != "" {
			if u, err := url.Parse(rawURL); err == nil {
				urls[name] = u
			}
		}
	}
	return urls
}