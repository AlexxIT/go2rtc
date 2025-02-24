package streams

import (
	"errors"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Handler func(source string) (core.Producer, error)

var handlers = map[string]Handler{}

func HandleFunc(scheme string, handler Handler) {
	handlers[scheme] = handler
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
			return handler(url)
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
