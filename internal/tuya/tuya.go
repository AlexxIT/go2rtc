package tuya

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tuya"
)

func Init() {
	streams.HandleFunc("tuya", func(source string) (core.Producer, error) {
		return tuya.Dial(source)
	})
}
