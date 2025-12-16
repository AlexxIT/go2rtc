package multitrans

import (
	"fmt"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/multitrans"
)

func Init() {
	streams.HandleFunc("multitrans", func(source string) (core.Producer, error) {
		fmt.Printf("[multitrans] handler called for source: %s\n", source)
		return multitrans.Dial(source)
	})
}
