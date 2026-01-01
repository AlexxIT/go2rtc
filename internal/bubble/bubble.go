package bubble

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/bubble"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func Init() {
	streams.HandleFunc("bubble", func(source string) (core.Producer, error) {
		return bubble.Dial(source)
	})
}
