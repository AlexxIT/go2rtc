//go:build !linux

package exec

import (
	"fmt"
	"net/url"
	"runtime"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/streams"
)

func parseParams(s string) (*Params, error) {
	args := &Params{
		Command: s,
	}

	var query url.Values
	if i := strings.IndexByte(s, '#'); i > 0 {
		query = streams.ParseQuery(s[i+1:])
		args.Command = s[:i]
	}

	if _, ok := query["killsignal"]; ok {
		return nil, fmt.Errorf("killsignal is not supported this %s", runtime.GOOS)
	}

	if _, ok := query["killtimeout"]; ok {
		return nil, fmt.Errorf("killtimeout is not supported in %s", runtime.GOOS)
	}

	return args, nil
}
