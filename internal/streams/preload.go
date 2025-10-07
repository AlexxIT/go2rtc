package streams

import (
	"errors"
	"net/url"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/probe"
)

var preloads = map[*Stream]*probe.Probe{}
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

	if cons := preloads[stream]; cons != nil {
		stream.RemoveConsumer(cons)
	}

	cons := probe.Create("preload", query)

	if err = stream.AddConsumer(cons); err != nil {
		return err
	}

	preloads[stream] = cons
	return nil
}

func DelPreload(stream *Stream) error {
	preloadsMu.Lock()
	defer preloadsMu.Unlock()

	if cons := preloads[stream]; cons != nil {
		stream.RemoveConsumer(cons)
		delete(preloads, stream)
		return nil
	}

	return errors.New("streams: preload not found")
}
