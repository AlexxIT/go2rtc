package dahua

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/dahua"
)

func Init() {
	streams.HandleFunc("dahua", func(source string) (core.Producer, error) {
		return dahua.Dial(source)
	})
}
