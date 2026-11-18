package tapo

import (
	"strings"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/kasa"
	"github.com/AlexxIT/go2rtc/pkg/tapo"
)

func Init() {
	streams.HandleFunc("kasa", func(source string) (core.Producer, error) {
		return kasa.Dial(source)
	})

	streams.HandleFunc("tapo", tapoHandler)
	streams.HandleFunc("vigi", tapoHandler)
}

func tapoHandler(rawURL string) (core.Producer, error) {
	rawURL, rawQuery, _ := strings.Cut(rawURL, "#")

	client, err := tapo.Dial(rawURL)
	if err != nil {
		return nil, err
	}
	if rawQuery != "" {
		query := streams.ParseQuery(rawQuery)
		client.Backchannel = query.Get("backchannel") == "1"
		client.Media = query.Get("media")
	}

	return client, nil
}
