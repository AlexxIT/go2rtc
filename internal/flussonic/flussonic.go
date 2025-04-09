package flussonic

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/flussonic"
)

func Init() {
	streams.HandleFunc("flussonic", flussonic.Dial)
}
