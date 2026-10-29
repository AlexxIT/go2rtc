package streams

import (
	"errors"
	"regexp"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/audio"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Handler func(source string) (core.Producer, error)

var handlers = map[string]Handler{}

func HandleFunc(scheme string, handler Handler) {
	handlers[scheme] = handler
}

func SupportedSchemes() []string {
	uniqueKeys := make(map[string]struct{}, len(handlers)+len(redirects))
	for scheme := range handlers {
		uniqueKeys[scheme] = struct{}{}
	}
	for scheme := range redirects {
		uniqueKeys[scheme] = struct{}{}
	}
	resultKeys := make([]string, 0, len(uniqueKeys))
	for key := range uniqueKeys {
		resultKeys = append(resultKeys, key)
	}
	return resultKeys
}

func HasProducer(url string) bool {
	if i := strings.IndexByte(url, ':'); i > 0 {
		scheme := url[:i]

		if _, ok := handlers[scheme]; ok {
			return true
		}

		if _, ok := redirects[scheme]; ok {
			return true
		}
	}

	return false
}

func GetProducer(url string) (core.Producer, error) {
	if i := strings.IndexByte(url, ':'); i > 0 {
		scheme := url[:i]

		if redirect, ok := redirects[scheme]; ok {
			location, err := redirect(url)
			if err != nil {
				return nil, err
			}
			if location != "" {
				return GetProducer(location)
			}
		}

		if handler, ok := handlers[scheme]; ok {
			prod, err := handler(url)
			if err != nil {
				return nil, err
			}
			return audio.WithProducer(prod), nil
		}
	}

	return nil, errors.New("streams: unsupported scheme: " + url)
}

// Redirect can return: location URL or error or empty URL and error
type Redirect func(url string) (string, error)

var redirects = map[string]Redirect{}

func RedirectFunc(scheme string, redirect Redirect) {
	redirects[scheme] = redirect
}

func Location(url string) (string, error) {
	if i := strings.IndexByte(url, ':'); i > 0 {
		scheme := url[:i]

		if redirect, ok := redirects[scheme]; ok {
			return redirect(url)
		}
	}

	return "", nil
}

// TODO: rework

type ConsumerHandler func(url string) (core.Consumer, func(), error)

var consumerHandlers = map[string]ConsumerHandler{}

func HandleConsumerFunc(scheme string, handler ConsumerHandler) {
	consumerHandlers[scheme] = handler
}

func GetConsumer(url string) (core.Consumer, func(), error) {
	if i := strings.IndexByte(url, ':'); i > 0 {
		scheme := url[:i]

		if handler, ok := consumerHandlers[scheme]; ok {
			return handler(url)
		}
	}

	return nil, nil, errors.New("streams: unsupported scheme: " + url)
}

var insecure = map[string]bool{}

func MarkInsecure(scheme string) {
	insecure[scheme] = true
}

var sanitize = regexp.MustCompile(`\s`)

func Validate(source string) error {
	// TODO: Review the entire logic of insecure sources
	if i := strings.IndexByte(source, ':'); i > 0 {
		if insecure[source[:i]] {
			return errors.New("streams: source from insecure producer")
		}
		if source[:i] == "ffmpeg" {
			if j := strings.IndexByte(source, '#'); j >= 0 {
				query := ParseQuery(source[j+1:])
				if query["raw"] != nil {
					return errors.New("streams: ffmpeg raw params may be insecure")
				}
			}
		}
	}
	if sanitize.MatchString(source) {
		return errors.New("streams: source with spaces may be insecure")
	}
	if strings.IndexAny(source, `'"`) >= 0 {
		return errors.New("streams: source with shell quotes may be insecure")
	}
	return nil
}
