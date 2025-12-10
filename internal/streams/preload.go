package streams

import (
	"errors"
	"net/url"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/probe"
)

type preload struct {
	cons  *probe.Probe
	query string
}

var preloads = map[*Stream]*preload{}
var preloadsMu sync.Mutex

func Preload(stream *Stream, rawQuery string) {
	if err := AddPreload(stream, rawQuery); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func AddPreload(stream *Stream, rawQuery string) error {
	if rawQuery == "" {
		rawQuery = "video&audio"
	}

	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return err
	}

	preloadsMu.Lock()
	defer preloadsMu.Unlock()

	if p := preloads[stream]; p != nil {
		stream.RemoveConsumer(p.cons)
	}

	cons := probe.Create("preload", query)

	if err = stream.AddConsumer(cons); err != nil {
		return err
	}

	preloads[stream] = &preload{cons: cons, query: rawQuery}
	return nil
}

func DelPreload(stream *Stream) error {
	preloadsMu.Lock()
	defer preloadsMu.Unlock()

	if p := preloads[stream]; p != nil {
		stream.RemoveConsumer(p.cons)
		delete(preloads, stream)
		return nil
	}

	return errors.New("streams: preload not found")
}

func GetPreloads() map[string]string {
	streamsMu.Lock()
	defer streamsMu.Unlock()

	preloadsMu.Lock()
	defer preloadsMu.Unlock()

	// build reverse lookup: stream -> name
	streamNames := make(map[*Stream]string, len(streams))
	for name, stream := range streams {
		streamNames[stream] = name
	}

	result := make(map[string]string, len(preloads))
	for stream, p := range preloads {
		if name, ok := streamNames[stream]; ok {
			result[name] = p.query
		}
	}
	return result
}
