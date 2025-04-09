package ivideon

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/ivideon"
)

func Init() {
	streams.HandleFunc("ivideon", ivideon.Dial)
}
