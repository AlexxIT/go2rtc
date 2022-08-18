package streams

import (
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"strings"
)

type Handler func(url string) (streamer.Producer, error)

var handlers map[string]Handler

func HandleFunc(scheme string, handler Handler) {
	if handlers == nil {
		handlers = make(map[string]Handler)
	}
	handlers[scheme] = handler
}

func HasProducer(url string) bool {
	i := strings.IndexByte(url, ':')
	return handlers[url[:i]] != nil
}

func GetProducer(url string) (streamer.Producer, error) {
	i := strings.IndexByte(url, ':')
	handler := handlers[url[:i]]
	if handler == nil {
		return nil, fmt.Errorf("unsupported scheme: %s", url)
	}
	return handler(url)
}
