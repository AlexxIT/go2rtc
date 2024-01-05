package outputbc

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/outputbc"
	"github.com/AlexxIT/go2rtc/pkg/shell"
)

func Init() {
	streams.HandleFunc("outputbc", handle)
}

func handle(url string) (core.Producer, error) {
	args := shell.QuoteSplit(url[9:])
	con, err := outputbc.NewClient(args)
	if err != nil {
		return nil, err
	}
	con.Dial()
	return con, nil
}
