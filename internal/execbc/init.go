package execbc

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/execbc"
	"github.com/AlexxIT/go2rtc/pkg/shell"
)

func Init() {
	streams.HandleFunc("execbc", handle)
}

func handle(url string) (core.Producer, error) {
	args := shell.QuoteSplit(url[7:])
	con, err := execbc.NewClient(args)
	if err != nil {
		return nil, err
	}
	con.Dial()
	return con, nil
}
