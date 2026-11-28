package isapi

import (
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	pkg "github.com/AlexxIT/go2rtc/pkg/isapi"
)

func Init() {
	pkg.SetLogger(app.GetLogger("isapi"))

	streams.HandleFunc("isapi", func(source string) (core.Producer, error) {
		return pkg.Dial(source)
	})
}
