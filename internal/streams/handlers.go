package streams

import (
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"strings"
	"sync"
)

type Handler func(url string) (core.Producer, error)

var handlers = map[string]Handler{}
var handlersMu sync.Mutex

func HandleFunc(scheme string, handler Handler) {
	handlersMu.Lock()
	handlers[scheme] = handler
	handlersMu.Unlock()
}

func getHandler(url string) Handler {
	i := strings.IndexByte(url, ':')
	if i <= 0 { // TODO: i < 4 ?
		return nil
	}
	handlersMu.Lock()
	defer handlersMu.Unlock()
	return handlers[url[:i]]
}

func HasProducer(url string) bool {
	return getHandler(url) != nil
}

func GetProducer(url string) (core.Producer, error) {
	handler := getHandler(url)
	if handler == nil {
		return nil, fmt.Errorf("unsupported scheme: %s", url)
	}
	return handler(url)
}
