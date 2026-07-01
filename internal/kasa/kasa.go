package kasa

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/kasa"
)

func Init() {
	streams.HandleFunc("kasa", func(source string) (core.Producer, error) {
		return kasa.Dial(source)
	})
}
