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

func getHandler(url string) Handler {
	i := strings.IndexByte(url, ':')
	if i <= 0 { // TODO: i < 4 ?
		return nil
	}
	return handlers[url[:i]]
}

func HasProducer(url string) bool {
	return getHandler(url) != nil
}

func GetProducer(url string) (streamer.Producer, error) {
	handler := getHandler(url)
	if handler == nil {
		return nil, fmt.Errorf("unsupported scheme: %s", url)
	}
	return handler(url)
}
