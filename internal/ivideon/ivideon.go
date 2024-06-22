package ivideon

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/ivideon"
)

func Init() {
	streams.HandleFunc("ivideon", func(source string) (core.Producer, error) {
		return ivideon.Dial(source)
	})
}
