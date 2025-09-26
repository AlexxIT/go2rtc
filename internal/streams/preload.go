package streams

import (
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/preload"
)

var preloads = map[string]*preload.Preload{}

func (s *Stream) Preload(name string, query url.Values) error {
	cons := preload.NewPreload(name, query)
	preloads[name] = cons

	if err := s.AddConsumer(cons); err != nil {
		return err
	}

	return nil
}

func Preload(src string, rawQuery string) {
	// skip if exists
	if _, ok := preloads[src]; ok {
		return
	}

	if stream := Get(src); stream != nil {
		query := ParseQuery(rawQuery)
		if err := stream.Preload(src, query); err != nil {
			log.Error().Err(err).Caller().Send()
		}
	}
}
