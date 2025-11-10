package ipeye

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/ipeye"
)

func Init() {
	streams.HandleFunc("ipeye", ipeye.Dial)
}
