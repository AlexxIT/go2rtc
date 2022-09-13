package echo

import (
	"bytes"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"os/exec"
)

func Init() {
	log := app.GetLogger("echo")

	streams.HandleFunc("echo", func(url string) (streamer.Producer, error) {
		args := shell.QuoteSplit(url[5:])

		b, err := exec.Command(args[0], args[1:]...).Output()
		if err != nil {
			return nil, err
		}

		b = bytes.TrimSpace(b)

		log.Debug().Str("url", url).Msgf("[echo] %s", b)

		return streams.GetProducer(string(b))
	})
}
