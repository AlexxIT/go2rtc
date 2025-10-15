package eseecloud

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/eseecloud"
)

func Init() {
	streams.HandleFunc("eseecloud", eseecloud.Dial)
}
