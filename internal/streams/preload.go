package streams

import (
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/preload"
)

func (s *Stream) Preload(query url.Values) error {
	cons := preload.NewPreload(query)

	if err := s.AddConsumer(cons); err != nil {
		return err
	}

	return nil
}

func Preload(src string) {
	name, rawQuery, _ := strings.Cut(src, "#")
	query := ParseQuery(rawQuery)

	if stream := Get(name); stream != nil {
		if err := stream.Preload(query); err != nil {
			log.Error().Err(err).Caller().Send()
		}
		return
	}
}
