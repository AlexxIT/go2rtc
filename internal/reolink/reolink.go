package reolink

import (
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/reolink"
)

func Init() {
	registry := reolink.NewRegistry(app.GetLogger("reolink"))
	streams.HandleFunc("reolink", func(source string) (core.Producer, error) {
		return registry.Dial(source)
	})
}
