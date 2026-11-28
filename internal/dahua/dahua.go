package dahua

import (
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/dahua"
)

func Init() {
	log := app.GetLogger("dahua")
	streams.HandleFunc("dahua", dahua.Dial)

	// Wire the pkg layer's raw-protocol dump into the zerolog "dahua" logger;
	// without this, debug=1 in the URL is a silent no-op.
	dahua.Trace = func(format string, v ...any) { log.Debug().Msgf(format, v...) }
}
