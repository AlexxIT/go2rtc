package multitrans

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/multitrans"
)

func Init() {
	streams.HandleFunc("multitrans", multitrans.Dial)
}
