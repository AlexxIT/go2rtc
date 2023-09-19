package device

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/api"
)

func Init(bin string) {
	Bin = bin

	api.HandleFunc("api/ffmpeg/devices", apiDevices)
}

func GetInput(src string) (string, error) {
	i := strings.IndexByte(src, '?')
	if i < 0 {
		return "", errors.New("empty query: " + src)
	}

	query, err := url.ParseQuery(src[i+1:])
	if err != nil {
		return "", err
	}

	runonce.Do(initDevices)

	if input := queryToInput(query); input != "" {
		return input, nil
	}

	return "", errors.New("wrong query: " + src)
}

var Bin string

var videos, audios []string
var streams []*api.Source
var runonce sync.Once

func apiDevices(w http.ResponseWriter, r *http.Request) {
	runonce.Do(initDevices)

	api.ResponseSources(w, streams)
}

func indexToItem(items []string, index string) string {
	if i, err := strconv.Atoi(index); err == nil && i < len(items) {
		return items[i]
	}
	return index
}
